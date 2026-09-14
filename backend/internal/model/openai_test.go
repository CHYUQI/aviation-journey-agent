package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return NewOpenAI(Options{
		BaseURL:     server.URL,
		APIKey:      "test-key",
		Model:       "qwen3-8b",
		Timeout:     5 * time.Second,
		Temperature: 0.1,
		HTTPClient:  server.Client(),
	})
}

func TestChat_SendsOpenAICompatibleRequest(t *testing.T) {
	var got map[string]any
	var auth string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"你好"}}]}`))
	})

	resp, err := client.Chat(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hi"}},
		JSONMode: true,
	})
	if err != nil {
		t.Fatalf("Chat 返回错误: %v", err)
	}
	if resp.Content != "你好" {
		t.Fatalf("content = %q", resp.Content)
	}
	if auth != "Bearer test-key" {
		t.Fatalf("Authorization = %q", auth)
	}
	if got["model"] != "qwen3-8b" {
		t.Fatalf("model = %v", got["model"])
	}
	if got["temperature"] != 0.1 {
		t.Fatalf("temperature = %v", got["temperature"])
	}
	if _, ok := got["response_format"]; !ok {
		t.Fatal("JSONMode=true 时应当带 response_format")
	}
}

func TestChat_HTTPErrorIncludesBody(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	})

	_, err := client.Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatal("期望报错，实际成功")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("错误信息里应包含状态码与响应体: %v", err)
	}
}

func TestChat_EmptyChoicesIsError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	})

	_, err := client.Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatal("choices 为空时期望报错")
	}
}

func TestChat_NotConfigured(t *testing.T) {
	client := NewOpenAI(Options{})
	if _, err := client.Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}}); err == nil {
		t.Fatal("端点未配置时期望报错，而不是发一个空请求")
	}
}

func TestGenerateJSON_RetriesOnInvalidJSON(t *testing.T) {
	calls := 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"好的，我来分析一下：这里不是JSON"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"risk\":\"unknown\"}"}}]}`))
	})

	var out struct {
		Risk string `json:"risk"`
	}
	if _, err := GenerateJSON(context.Background(), client, Request{
		Messages: []Message{{Role: "user", Content: "给个建议"}},
	}, &out); err != nil {
		t.Fatalf("GenerateJSON 返回错误: %v", err)
	}
	if calls != 2 {
		t.Fatalf("期望重试一次，实际调用 %d 次", calls)
	}
	if out.Risk != "unknown" {
		t.Fatalf("risk = %q", out.Risk)
	}
}

func TestGenerateJSON_FailsAfterTwoBadAnswers(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"还是不是JSON"}}]}`))
	})

	var out map[string]any
	_, err := GenerateJSON(context.Background(), client, Request{
		Messages: []Message{{Role: "user", Content: "给个建议"}},
	}, &out)
	if err == nil {
		t.Fatal("两次都不是 JSON 时期望报错，调用方需要据此降级")
	}
}

func TestUnmarshalJSON_ToleratesMarkdownFenceAndProse(t *testing.T) {
	cases := map[string]string{
		"纯 JSON":      `{"risk":"red"}`,
		"markdown 包裹": "```json\n{\"risk\":\"red\"}\n```",
		"前后有说明":       "分析结果如下：{\"risk\":\"red\"} 以上。",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			var out struct {
				Risk string `json:"risk"`
			}
			if err := unmarshalJSON(content, &out); err != nil {
				t.Fatalf("unmarshalJSON 失败: %v", err)
			}
			if out.Risk != "red" {
				t.Fatalf("risk = %q", out.Risk)
			}
		})
	}
}
