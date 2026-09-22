// Package config 负责从环境变量（含 .env 文件）读取运行配置。
//
// 约定：所有可变化的配置都走环境变量，代码里不写死。
// 换机器部署时只改 .env，不用重新编译。
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// EnvFileEnvVar 用于显式指定 .env 路径；设置后不再走默认查找。
const EnvFileEnvVar = "AJA_ENV_FILE"

// Config 是服务的全部运行配置。
type Config struct {
	Addr string
	// EnvFile 是实际加载的 .env 路径；空字符串表示没有找到 .env。
	EnvFile string
	Model   ModelConfig
}

// ModelConfig 是模型服务配置。
//
// 三种部署方式（阿里云百炼 / vLLM / Ollama）都走 OpenAI 兼容协议，
// 切换只需改 BaseURL、APIKey、Name 三项。
type ModelConfig struct {
	BaseURL     string
	APIKey      string
	Name        string
	Timeout     time.Duration
	Temperature float64
	// MaxConcurrency 是同进程内模型调用的并发上限。
	// 默认 1（串行）：实测并发调用会让百炼端点长时间不响应直到超时。
	MaxConcurrency int
}

// Configured 表示模型配置是否完整。缺配置时上层应当走降级路径，
// 而不是拿一个空 key 去请求。
func (m ModelConfig) Configured() bool {
	return len(m.MissingFields()) == 0
}

// MissingFields 返回还空着的必填环境变量名，用于诊断输出。
func (m ModelConfig) MissingFields() []string {
	var missing []string
	if m.BaseURL == "" {
		missing = append(missing, "MODEL_BASE_URL")
	}
	if m.APIKey == "" {
		missing = append(missing, "MODEL_API_KEY")
	}
	if m.Name == "" {
		missing = append(missing, "MODEL_NAME")
	}
	return missing
}

// Load 读取配置。会先按 LoadDefaultEnvFile 的规则加载 .env，再读环境变量；
// 已存在的环境变量优先级更高，不会被 .env 覆盖。
func Load() Config {
	envFile := LoadDefaultEnvFile()

	return Config{
		Addr:    getenv("SERVER_ADDR", ":8080"),
		EnvFile: envFile,
		Model: ModelConfig{
			BaseURL:        strings.TrimRight(os.Getenv("MODEL_BASE_URL"), "/"),
			APIKey:         os.Getenv("MODEL_API_KEY"),
			Name:           getenv("MODEL_NAME", "qwen3-8b"),
			Timeout:        getduration("MODEL_TIMEOUT", 90*time.Second),
			Temperature:    getfloat("MODEL_TEMPERATURE", 0.1),
			MaxConcurrency: getint("MODEL_MAX_CONCURRENCY", 1),
		},
	}
}

// LoadDefaultEnvFile 查找并加载 .env，返回实际加载的文件路径；
// 没找到时返回空字符串。已存在的环境变量不会被 .env 覆盖。
//
// 查找不依赖启动目录：
//  1. 环境变量 AJA_ENV_FILE 指定的文件（设置后不再回退）；
//  2. 从当前工作目录逐级向上，每级先看 backend/.env 再看 .env，
//     直到包含 go.mod 或 backend/go.mod 的项目根为止；
//  3. 从可执行文件所在目录向上做同样的查找。
func LoadDefaultEnvFile() string {
	if path := strings.TrimSpace(os.Getenv(EnvFileEnvVar)); path != "" {
		if isRegularFile(path) {
			_ = LoadEnvFile(path)
			return path
		}
		return ""
	}

	for _, path := range envFileCandidates() {
		if isRegularFile(path) {
			_ = LoadEnvFile(path)
			return path
		}
	}
	return ""
}

// envFileCandidates 按优先级列出候选 .env 路径，只用于查找，不保证存在。
func envFileCandidates() []string {
	var dirs []string
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, projectDirsUp(wd)...)
	}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, projectDirsUp(filepath.Dir(exe))...)
	}

	seenDir := make(map[string]bool)
	seenPath := make(map[string]bool)
	var paths []string
	for _, dir := range dirs {
		if seenDir[dir] {
			continue
		}
		seenDir[dir] = true

		for _, rel := range []string{filepath.Join("backend", ".env"), ".env"} {
			path := filepath.Join(dir, rel)
			if seenPath[path] {
				continue
			}
			seenPath[path] = true
			paths = append(paths, path)
		}
	}
	return paths
}

// projectDirsUp 返回从 start 到项目根（含）的目录；找不到项目标记时
// 只返回 start 自己，避免往上捡到无关目录里的 .env。
func projectDirsUp(start string) []string {
	dirs := dirsUpToRoot(start)
	for i, dir := range dirs {
		if isProjectRoot(dir) {
			return dirs[:i+1]
		}
	}
	return dirs[:1]
}

func dirsUpToRoot(dir string) []string {
	var dirs []string
	for {
		dirs = append(dirs, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			return dirs
		}
		dir = parent
	}
}

// isProjectRoot 判断目录是不是本仓库的根：模块根（go.mod）或仓库根
// （backend/go.mod）。用它把 .env 查找范围限制在项目内。
func isProjectRoot(dir string) bool {
	return isRegularFile(filepath.Join(dir, "go.mod")) ||
		isRegularFile(filepath.Join(dir, "backend", "go.mod"))
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// LoadEnvFile 把 .env 里的键值对注入环境变量。
//
// 只支持 KEY=VALUE、# 注释和空行，够用即可，不引第三方依赖。
// 文件不存在时返回 nil —— .env 是可选的。
func LoadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)

		// 已经存在的环境变量优先，不覆盖
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}

	return scanner.Err()
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getduration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

// getint 读一个整数配置，读不到或格式不对就用默认值。
func getint(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getfloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}
