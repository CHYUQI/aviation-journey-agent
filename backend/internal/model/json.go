package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// GenerateJSON 要求模型输出 JSON，并解析到 out。
//
// 模型输出不可信：解析失败时把错误回灌给模型重试一次，
// 仍然失败就返回错误，由调用方决定降级（通常是留 null + 降低 confidence）。
//
// 注意：这里只保证"是合法 JSON 且能填进结构体"。
// 枚举白名单、必填字段、业务规则等校验必须由调用方再做一遍。
func GenerateJSON(ctx context.Context, client Client, req Request, out any) (Response, error) {
	req.JSONMode = true

	resp, err := client.Chat(ctx, req)
	if err != nil {
		return Response{}, err
	}

	if err := unmarshalJSON(resp.Content, out); err != nil {
		return retryJSON(ctx, client, req, resp, err, out)
	}
	return resp, nil
}

func retryJSON(ctx context.Context, client Client, req Request, first Response, cause error, out any) (Response, error) {
	retry := req
	retry.Messages = append(
		append([]Message{}, req.Messages...),
		Message{Role: "assistant", Content: first.Content},
		Message{
			Role:    "user",
			Content: "上面的输出不是合法 JSON（" + cause.Error() + "）。请只输出 JSON 对象，不要解释、不要 markdown 代码块。",
		},
	)

	resp, err := client.Chat(ctx, retry)
	if err != nil {
		return Response{}, err
	}
	if err := unmarshalJSON(resp.Content, out); err != nil {
		return Response{}, fmt.Errorf("模型两次都没有返回合法 JSON: %w", err)
	}
	// 搜索场景下，重试那一轮的检索结果同样有效
	if len(resp.Sources) == 0 {
		resp.Sources = first.Sources
	}
	return resp, nil
}

// unmarshalJSON 容忍常见的模型输出毛病：
// 包了一层 markdown 代码块，或者 JSON 前后带了说明文字。
func unmarshalJSON(content string, out any) error {
	raw := strings.TrimSpace(content)
	if raw == "" {
		return fmt.Errorf("模型返回了空内容")
	}

	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```JSON")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
		raw = strings.TrimSpace(raw)
	}

	if start := strings.Index(raw, "{"); start > 0 {
		raw = raw[start:]
	}
	if end := strings.LastIndex(raw, "}"); end >= 0 && end < len(raw)-1 {
		raw = raw[:end+1]
	}

	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("解析 JSON 失败: %w", err)
	}
	return nil
}
