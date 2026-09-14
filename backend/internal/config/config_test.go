package config

import (
	"os"
	"path/filepath"
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
		{"完整", ModelConfig{BaseURL: "http://x/v1", Name: "qwen3-8b"}, true},
		{"缺端点", ModelConfig{Name: "qwen3-8b"}, false},
		{"缺模型名", ModelConfig{BaseURL: "http://x/v1"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Configured(); got != tc.want {
				t.Fatalf("Configured() = %v, want %v", got, tc.want)
			}
		})
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
