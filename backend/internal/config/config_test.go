package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# 注释\nMODEL_NAME=qwen3-8b\nMODEL_TIMEOUT=30s\nQUOTED=\"带引号的值\"\n\nNO_EQUALS_LINE\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MODEL_NAME", "already-set")
	t.Setenv("MODEL_TIMEOUT", "")
	_ = os.Unsetenv("MODEL_TIMEOUT")
	_ = os.Unsetenv("QUOTED")

	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile 失败: %v", err)
	}

	if got := os.Getenv("MODEL_NAME"); got != "already-set" {
		t.Fatalf("已存在的环境变量不应被 .env 覆盖，实际 %q", got)
	}
	if got := os.Getenv("MODEL_TIMEOUT"); got != "30s" {
		t.Fatalf("MODEL_TIMEOUT = %q", got)
	}
	if got := os.Getenv("QUOTED"); got != "带引号的值" {
		t.Fatalf("引号应被去掉，实际 %q", got)
	}
}

func TestLoadEnvFile_MissingFileIsNotAnError(t *testing.T) {
	if err := LoadEnvFile(filepath.Join(t.TempDir(), "nope.env")); err != nil {
		t.Fatalf(".env 不存在时不应报错，实际 %v", err)
	}
}

func TestModelConfig_Configured(t *testing.T) {
	cases := []struct {
		name string
		cfg  ModelConfig
		want bool
	}{
		{"完整", ModelConfig{BaseURL: "http://x/v1", APIKey: "sk-x", Name: "qwen3-8b"}, true},
		{"缺端点", ModelConfig{APIKey: "sk-x", Name: "qwen3-8b"}, false},
		{"缺模型名", ModelConfig{BaseURL: "http://x/v1", APIKey: "sk-x"}, false},
		{"缺 key", ModelConfig{BaseURL: "http://x/v1", Name: "qwen3-8b"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Configured(); got != tc.want {
				t.Fatalf("Configured() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestModelConfig_MissingFields(t *testing.T) {
	got := ModelConfig{}.MissingFields()
	want := []string{"MODEL_BASE_URL", "MODEL_API_KEY", "MODEL_NAME"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MissingFields() = %v, want %v", got, want)
	}
}

// 复现“在 backend/cmd/server 下 go run main.go”的场景：
// .env 在 backend/ 下，也必须能被向上查找到。
func TestLoadDefaultEnvFile_FindsBackendEnvFromSubdir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "backend", "go.mod"), "module example\n")
	envPath := filepath.Join(root, "backend", ".env")
	writeFile(t, envPath, "MODEL_API_KEY=from-file\n")

	sub := filepath.Join(root, "backend", "cmd", "server")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, sub)
	unsetenv(t, EnvFileEnvVar)
	unsetenv(t, "MODEL_API_KEY")

	if got := LoadDefaultEnvFile(); !samePath(got, envPath) {
		t.Fatalf("LoadDefaultEnvFile() = %q, want %q", got, envPath)
	}
	if got := os.Getenv("MODEL_API_KEY"); got != "from-file" {
		t.Fatalf("MODEL_API_KEY = %q, want from-file", got)
	}
}

func TestLoadDefaultEnvFile_ExplicitPathWins(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), "custom.env")
	writeFile(t, envPath, "MODEL_NAME=from-custom\n")

	t.Setenv(EnvFileEnvVar, envPath)
	unsetenv(t, "MODEL_NAME")

	if got := LoadDefaultEnvFile(); !samePath(got, envPath) {
		t.Fatalf("LoadDefaultEnvFile() = %q, want %q", got, envPath)
	}
	if got := os.Getenv("MODEL_NAME"); got != "from-custom" {
		t.Fatalf("MODEL_NAME = %q, want from-custom", got)
	}
}

func TestLoadDefaultEnvFile_MissingExplicitPathDoesNotFallBack(t *testing.T) {
	t.Setenv(EnvFileEnvVar, filepath.Join(t.TempDir(), "nope.env"))
	if got := LoadDefaultEnvFile(); got != "" {
		t.Fatalf("显式路径不存在时应返回空，实际 %q", got)
	}
}

func TestGetDurationAndFloatFallbacks(t *testing.T) {
	t.Setenv("X_TIMEOUT", "不是时长")
	if got := getduration("X_TIMEOUT", 45*time.Second); got != 45*time.Second {
		t.Fatalf("非法时长应回退默认值，实际 %v", got)
	}
	t.Setenv("X_TEMP", "abc")
	if got := getfloat("X_TEMP", 0.1); got != 0.1 {
		t.Fatalf("非法数字应回退默认值，实际 %v", got)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

// unsetenv 清掉环境变量，并在测试结束后恢复原值。
func unsetenv(t *testing.T, key string) {
	t.Helper()
	old, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, old)
			return
		}
		_ = os.Unsetenv(key)
	})
}

// samePath 比较路径，Windows 下大小写不敏感。
func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return strings.EqualFold(filepath.Clean(absA), filepath.Clean(absB))
}
