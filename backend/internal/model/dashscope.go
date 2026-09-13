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

// DashScopeOptions 是百炼原生 API 客户端的配置。
//
// 为什么单独写一个客户端：实测 OpenAI 兼容端点不透传 enable_search，
// 联网检索只能走原生端点。原生端点还会把命中的网页放在
// output.search_info.search_results 里返回 —— 这正是校验
// "模型有没有引用真实来源"的依据。
type DashScopeOptions struct {
	BaseURL string // 例如 https://dashscope.aliyuncs.com/api/v1
	APIKey  string
	// Model 是不带联网检索时使用的模型
	Model string
	// SearchModel 是需要联网检索时使用的模型。
	// 不同模型对 enable_search 的支持不一样，实测 glm 系列不支持。
	SearchModel string
	Timeout     time.Duration
	Temperature float64
	HTTPClient  *http.Client
}

type dashScopeClient struct {
	baseURL     string
	apiKey      string
	model       string
	searchModel string
	temperature float64
	http        *http.Client
}

// NewDashScope 构造百炼原生 API 客户端。
func NewDashScope(opts DashScopeOptions) Client {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		timeout := opts.Timeout
		if timeout <= 0 {
			timeout = 45 * time.Second
		}
		httpClient = &http.Client{Timeout: timeout}
	}

	searchModel := opts.SearchModel
	if searchModel == "" {
		searchModel = opts.Model
	}

	return &dashScopeClient{
		baseURL:     strings.TrimRight(opts.BaseURL, "/"),
		apiKey:      opts.APIKey,
		model:       opts.Model,
		searchModel: searchModel,
		temperature: opts.Temperature,
		http:        httpClient,
	}
}

func (c *dashScopeClient) Chat(ctx context.Context, req Request) (Response, error) {
	if c.baseURL == "" {
		return Response{}, errors.New("百炼原生端点未配置")
	}
	if len(req.Messages) == 0 {
		return Response{}, errors.New("模型请求缺少消息")
	}

	model := c.model
	if req.Search {
		model = c.searchModel
	}
	if req.Model != "" {
		model = req.Model
	}

	temperature := c.temperature
	if req.Temperature != nil {
		temperature = *req.Temperature
	}

	parameters := map[string]any{
		"result_format": "message",
		"temperature":   temperature,
	}
	if req.Search {
		parameters["enable_search"] = true
		parameters["search_options"] = map[string]any{
			"enable_source": true,
			"forced_search": true,
		}
	}

	payload := map[string]any{
		"model":      model,
		"input":      map[string]any{"messages": req.Messages},
		"parameters": parameters,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, fmt.Errorf("序列化模型请求失败: %w", err)
	}

	url := c.baseURL + "/services/aigc/text-generation/generation"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("构造模型请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("调用模型失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Response{}, fmt.Errorf("模型返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}

	var parsed dashScopeResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Response{}, fmt.Errorf("解析模型响应失败: %w", err)
	}
	if parsed.Code != "" {
		return Response{}, fmt.Errorf("模型返回错误 %s: %s", parsed.Code, parsed.Message)
	}
	if len(parsed.Output.Choices) == 0 {
		return Response{}, errors.New("模型没有返回任何结果")
	}

	result := Response{Content: parsed.Output.Choices[0].Message.Content}
	for _, item := range parsed.Output.SearchInfo.SearchResults {
		result.Sources = append(result.Sources, Source{
			Title: item.Title,
			Site:  item.SiteName,
			URL:   item.URL,
		})
	}
	return result, nil
}

type dashScopeResponse struct {
	Output struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		SearchInfo struct {
			SearchResults []struct {
				Title    string `json:"title"`
				SiteName string `json:"site_name"`
				URL      string `json:"url"`
			} `json:"search_results"`
		} `json:"search_info"`
	} `json:"output"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
