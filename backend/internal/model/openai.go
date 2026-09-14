package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Options 是构建 OpenAI 兼容客户端所需的配置。
type Options struct {
	BaseURL     string
	APIKey      string
	Model       string
	Timeout     time.Duration
	Temperature float64
	// HTTPClient 可注入，便于测试；为 nil 时按 Timeout 新建
	HTTPClient *http.Client
}

type openAIClient struct {
	baseURL     string
	apiKey      string
	model       string
	temperature float64
	http        *http.Client
}

// NewOpenAI 构造 OpenAI 兼容客户端。
// 百炼、vLLM、Ollama 通用，差别只在 Options 里的端点与模型名。
func NewOpenAI(opts Options) Client {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		timeout := opts.Timeout
		if timeout <= 0 {
			timeout = 45 * time.Second
		}
		httpClient = &http.Client{Timeout: timeout}
	}

	return &openAIClient{
		baseURL:     strings.TrimRight(opts.BaseURL, "/"),
		apiKey:      opts.APIKey,
		model:       opts.Model,
		temperature: opts.Temperature,
		http:        httpClient,
	}
}

func (c *openAIClient) Chat(ctx context.Context, req Request) (Response, error) {
	if c.baseURL == "" {
		return Response{}, errors.New("模型端点未配置")
	}
	if len(req.Messages) == 0 {
		return Response{}, errors.New("模型请求缺少消息")
	}

	temperature := c.temperature
	if req.Temperature != nil {
		temperature = *req.Temperature
	}

	model := c.model
	if req.Model != "" {
		model = req.Model
	}

	payload := chatPayload{
		Model:       model,
		Messages:    req.Messages,
		Temperature: temperature,
	}
	if req.JSONMode {
		payload.ResponseFormat = map[string]any{"type": "json_object"}
	}
	if len(req.Tools) > 0 {
		payload.Tools = toToolWire(req.Tools)
	}

	body, err := encodePayload(payload, req.Extra)
	if err != nil {
		return Response{}, err
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("构造模型请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("调用模型失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Response{}, fmt.Errorf("模型返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}

	var parsed chatCompletion
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Response{}, fmt.Errorf("解析模型响应失败: %w", err)
	}
	if parsed.Error != nil {
		return Response{}, fmt.Errorf("模型返回错误: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return Response{}, errors.New("模型没有返回任何结果")
	}

	message := parsed.Choices[0].Message
	return Response{Content: message.Content, ToolCalls: message.ToolCalls}, nil
}

func toToolWire(defs []ToolDef) []toolWire {
	wire := make([]toolWire, 0, len(defs))
	for _, d := range defs {
		var item toolWire
		item.Type = "function"
		item.Function.Name = d.Name
		item.Function.Description = d.Description
		item.Function.Parameters = d.Parameters
		wire = append(wire, item)
	}
	return wire
}

// ---------- 线上格式 ----------

type chatPayload struct {
	Model          string     `json:"model"`
	Messages       []Message  `json:"messages"`
	Temperature    float64    `json:"temperature"`
	ResponseFormat any        `json:"response_format,omitempty"`
	Tools          []toolWire `json:"tools,omitempty"`
}

type toolWire struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type chatCompletion struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`

	// 有些兼容端点会用 200 返回错误体
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// encodePayload 序列化请求体。Extra 非空时先转成 map 再合并，
// 这样各家服务端的私有参数（比如百炼的 enable_search）能透传进去，
// 而不用为每个厂商改一次结构体。
func encodePayload(payload chatPayload, extra map[string]any) ([]byte, error) {
	if len(extra) == 0 {
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("序列化模型请求失败: %w", err)
		}
		return body, nil
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化模型请求失败: %w", err)
	}
	merged := map[string]any{}
	if err := json.Unmarshal(raw, &merged); err != nil {
		return nil, fmt.Errorf("合并透传参数失败: %w", err)
	}
	for k, v := range extra {
		merged[k] = v
	}

	body, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("序列化模型请求失败: %w", err)
	}
	return body, nil
}
