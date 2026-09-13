// Package config 负责从环境变量（含 .env 文件）读取运行配置。
//
// 约定：所有可变化的配置都走环境变量，代码里不写死。
// 换机器部署时只改 .env，不用重新编译。
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 是服务的全部运行配置。
type Config struct {
	Addr   string
	Model  ModelConfig
	Search SearchConfig
}

// SearchConfig 是联网检索的配置。
//
// 检索必须走百炼原生端点：实测 OpenAI 兼容端点不透传 enable_search，
// 参数被静默忽略，模型会退回"凭记忆回答"，从而编造内容。
// BaseURL 留空表示不启用检索。
type SearchConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

// Configured 表示检索通道配置完整。
func (s SearchConfig) Configured() bool {
	return s.BaseURL != "" && s.APIKey != "" && s.Model != ""
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
}

// Configured 表示模型配置是否完整。缺配置时上层应当走降级路径，
// 而不是拿一个空 key 去请求。
func (m ModelConfig) Configured() bool {
	return m.BaseURL != "" && m.Name != ""
}

// Load 读取配置。会先尝试加载同目录下的 .env，再读环境变量；
// 已存在的环境变量优先级更高，不会被 .env 覆盖。
func Load() Config {
	_ = LoadEnvFile(".env")

	return Config{
		Addr: getenv("SERVER_ADDR", ":8080"),
		Search: SearchConfig{
			BaseURL: os.Getenv("MODEL_NATIVE_URL"),
			APIKey:  os.Getenv("MODEL_API_KEY"),
			Model:   getenv("MODEL_SEARCH_NAME", "qwen-plus"),
			Timeout: getduration("MODEL_SEARCH_TIMEOUT", 60*time.Second),
		},
		Model: ModelConfig{
			BaseURL:     strings.TrimRight(os.Getenv("MODEL_BASE_URL"), "/"),
			APIKey:      os.Getenv("MODEL_API_KEY"),
			Name:        getenv("MODEL_NAME", "qwen3-8b"),
			Timeout:     getduration("MODEL_TIMEOUT", 45*time.Second),
			Temperature: getfloat("MODEL_TEMPERATURE", 0.1),
		},
	}
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
