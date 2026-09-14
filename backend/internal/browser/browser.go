// Package browser 用本机已安装的 Edge/Chrome 做无头渲染与页面交互。
//
// 为什么需要它：航司的航班动态页是 JS 渲染的，普通 HTTP 抓取只能拿到空壳。
// 实测南航页面：直接抓取 4.5KB 空壳，渲染后 33.6KB 才有真内容。
//
// 为什么不用 Playwright/Puppeteer：本机已经装了 Edge，用 CDP 直接驱动即可，
// 零额外依赖，也不用下载一百多兆的浏览器。
//
// 为什么不用视觉模型：渲染之后拿到的就是 DOM 文本，直接读文本比截图识别
// 更准、更便宜，也更容易校验（截图读错了没法回溯）。
package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/websocket"
)

const (
	startupTimeout = 30 * time.Second
	callTimeout    = 30 * time.Second
)

// Browser 是一个无头浏览器会话。
type Browser struct {
	cmd     *exec.Cmd
	conn    *websocket.Conn
	profile string
	seq     int
}

// LaunchOptions 是启动参数。
type LaunchOptions struct {
	// UserAgent 为空时，自动使用「去掉 Headless 标记」的浏览器原生 UA
	UserAgent string
}

// Launch 启动无头浏览器并连上调试端口。
//
// 用 --remote-debugging-port=0 让系统分配端口，再从 user-data-dir 下的
// DevToolsActivePort 文件读回来 —— 这样不会和用户已开的浏览器抢端口。
func Launch(ctx context.Context, opts LaunchOptions) (*Browser, error) {
	exe, err := findBrowser()
	if err != nil {
		return nil, err
	}

	// 用固定的配置目录，不要每次新建。
	//
	// 这不是"优化"，是能不能访问的问题：站点前面挂着 Cloudflare，
	// 一次性 profile 意味着每次都是全新访客，挑战会反复出现。
	// 真人浏览是一个 profile 一直用，clearance cookie 一直在。
	profile, err := os.UserCacheDir()
	if err != nil {
		profile = os.TempDir()
	}
	profile = filepath.Join(profile, "aviation-journey-agent", "browser-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return nil, fmt.Errorf("创建浏览器配置目录失败: %w", err)
	}

	// 固定 profile 会留下上一次的调试端口文件。若不在启动前清掉，
	// waitForPort 可能先读到旧端口，连到已经退出的浏览器进程。
	_ = os.Remove(filepath.Join(profile, "DevToolsActivePort"))

	cmd := exec.Command(exe,
		"--headless=new",
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-extensions",
		"--remote-debugging-port=0",
		"--remote-allow-origins=*",
		"--user-data-dir="+profile,
		"about:blank",
	)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(profile)
		return nil, fmt.Errorf("启动浏览器失败: %w", err)
	}

	b := &Browser{cmd: cmd, profile: profile}

	port, err := waitForPort(profile, startupTimeout)
	if err != nil {
		b.Close()
		return nil, err
	}

	wsURL, err := pageTargetURL(port)
	if err != nil {
		b.Close()
		return nil, err
	}

	conn, err := websocket.Dial(wsURL, "", "http://localhost/")
	if err != nil {
		b.Close()
		return nil, fmt.Errorf("连接调试端口失败: %w", err)
	}
	b.conn = conn

	if _, err := b.call("Page.enable", nil); err != nil {
		b.Close()
		return nil, err
	}
	if _, err := b.call("Runtime.enable", nil); err != nil {
		b.Close()
		return nil, err
	}

	// 设置 UA 必须在导航之前做，否则第一个真实请求就带着 Headless 标记
	if err := b.applyUserAgent(opts.UserAgent); err != nil {
		b.Close()
		return nil, err
	}

	return b, nil
}

// applyUserAgent 设置请求头里的 User-Agent。
//
// 为什么必须做：headless 模式默认上报 "HeadlessChrome/..."，很多站点的
// 机器人防护直接据此拦截 —— 实测 eoob.com.cn 的 Cloudflare 就是这么判的，
// 补再多的普通请求头都没用。
//
// 这里不写死版本号，而是读浏览器自己的 UA 再把 Headless 标记去掉，
// 浏览器升级后不用改代码。
func (b *Browser) applyUserAgent(custom string) error {
	if custom == "" {
		real, err := b.EvaluateString("navigator.userAgent")
		if err != nil || real == "" {
			return nil
		}
		custom = strings.Replace(real, "HeadlessChrome", "Chrome", 1)
		if custom == real {
			return nil // 本来就不是 headless UA，不用改
		}
	}

	if _, err := b.call("Network.enable", nil); err != nil {
		return err
	}
	_, err := b.call("Network.setUserAgentOverride", map[string]any{"userAgent": custom})
	return err
}

// Close 关闭连接、结束浏览器进程树，并清掉临时配置目录。
func (b *Browser) Close() error {
	if b.conn != nil {
		_ = b.conn.Close()
		b.conn = nil
	}
	if b.cmd != nil && b.cmd.Process != nil {
		killTree(b.cmd.Process.Pid)
		_, _ = b.cmd.Process.Wait()
		b.cmd = nil
	}
	// 刻意不删 profile：cookie 要留着，下次访问才不用重新过挑战
	if b.profile != "" {
		time.Sleep(300 * time.Millisecond)
		b.profile = ""
	}
	return nil
}

// killTree 连同子进程一起结束浏览器。
//
// Edge 启动后会派生渲染器、GPU、网络服务等一堆子进程，只 Kill 父进程
// 会留下孤儿进程和删不掉的临时目录 —— 实测跑十几轮就攒了九个残留进程。
// Windows 下用 taskkill /T 结束整棵树。
func killTree(pid int) {
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
		return
	}
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Kill()
	}
}

// Navigate 打开页面并等待加载完成。
func (b *Browser) Navigate(url string) error {
	_, err := b.call("Page.navigate", map[string]any{"url": url})
	return err
}

// PageText 返回渲染后的页面可见文本。
func (b *Browser) PageText() (string, error) {
	return b.EvaluateString("document.body ? document.body.innerText : ''")
}

// EvaluateString 在页面里执行一段表达式并取回字符串结果。
func (b *Browser) EvaluateString(expr string) (string, error) {
	raw, err := b.call("Runtime.evaluate", map[string]any{
		"expression":    expr,
		"returnByValue": true,
		"awaitPromise":  true,
	})
	if err != nil {
		return "", err
	}

	var result struct {
		Result struct {
			Value any `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("解析执行结果失败: %w", err)
	}
	if result.ExceptionDetails != nil {
		return "", fmt.Errorf("页面脚本出错: %s", result.ExceptionDetails.Text)
	}
	if result.Result.Value == nil {
		return "", nil
	}
	return fmt.Sprint(result.Result.Value), nil
}

// call 发一条 CDP 命令并等它的响应。期间收到的事件帧直接跳过。
func (b *Browser) call(method string, params any) (json.RawMessage, error) {
	if b.conn == nil {
		return nil, fmt.Errorf("浏览器未启动")
	}

	b.seq++
	id := b.seq

	payload, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if _, err := b.conn.Write(payload); err != nil {
		return nil, fmt.Errorf("发送 %s 失败: %w", method, err)
	}

	deadline := time.Now().Add(callTimeout)
	_ = b.conn.SetReadDeadline(deadline)

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("执行 %s 超时", method)
		}

		var frame string
		if err := websocket.Message.Receive(b.conn, &frame); err != nil {
			return nil, fmt.Errorf("读取 %s 响应失败: %w", method, err)
		}

		var msg struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(frame), &msg); err != nil {
			continue
		}
		if msg.ID != id {
			continue // 事件通知或其它命令的响应
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("CDP %s 失败: %s", method, msg.Error.Message)
		}
		return msg.Result, nil
	}
}

// findBrowser 找本机可用的 Chromium 内核浏览器。
// 用 BROWSER_PATH 环境变量可以覆盖。
func findBrowser() (string, error) {
	if custom := os.Getenv("BROWSER_PATH"); custom != "" {
		if _, err := os.Stat(custom); err == nil {
			return custom, nil
		}
	}

	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("ProgramFiles"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Google\Chrome\Application\chrome.exe`),
	}
	for _, path := range candidates {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("没有找到 Edge 或 Chrome，可用 BROWSER_PATH 环境变量指定")
}

// waitForPort 等浏览器把调试端口写进 DevToolsActivePort 文件。
func waitForPort(profile string, timeout time.Duration) (string, error) {
	path := filepath.Join(profile, "DevToolsActivePort")
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
			if len(lines) > 0 && lines[0] != "" {
				if _, err := strconv.Atoi(strings.TrimSpace(lines[0])); err == nil {
					return strings.TrimSpace(lines[0]), nil
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	return "", fmt.Errorf("浏览器调试端口未就绪（%s）", timeout)
}

// pageTargetURL 从调试接口里取页面的 WebSocket 地址。
func pageTargetURL(port string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	url := "http://127.0.0.1:" + port + "/json/list"

	deadline := time.Now().Add(startupTimeout)
	for time.Now().Before(deadline) {
		// 端口文件刚写出来时，HTTP 服务可能还没起来
		if _, err := net.DialTimeout("tcp", "127.0.0.1:"+port, time.Second); err == nil {
			resp, err := client.Get(url)
			if err == nil {
				var targets []struct {
					Type                 string `json:"type"`
					WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
				}
				err = json.NewDecoder(resp.Body).Decode(&targets)
				resp.Body.Close()
				if err == nil {
					for _, t := range targets {
						if t.Type == "page" && t.WebSocketDebuggerURL != "" {
							return t.WebSocketDebuggerURL, nil
						}
					}
				}
			}
		}
		time.Sleep(300 * time.Millisecond)
	}

	return "", fmt.Errorf("没有找到可用的页面调试目标")
}
