// Package model 是模型服务的统一入口。
//
// 阿里云百炼、本地 vLLM、Ollama 都遵循 OpenAI 兼容协议，
// 因此只需要一个实现，靠配置切换端点（见 .env.example）。
package model

import "context"

// Message 是对话中的一条消息。
type Message struct {
	Role    string `json:"role"` // system | user | assistant | tool
	Content string `json:"content"`

	// ToolCalls 只出现在 assistant 消息里
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolCallID 只出现在 tool 消息里，指向它所回应的那次调用
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ToolCall 是模型请求调用某个工具。
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON 字符串
	} `json:"function"`
}

// ToolDef 是提供给模型的工具定义。
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// Request 是一次模型调用。
type Request struct {
	Messages []Message

	// Model 为空时使用客户端默认模型。
	// 用它可以让不同 Agent 用不同模型：取数用快模型，决策用强模型。
	Model string

	// Temperature 为 nil 时使用客户端默认温度
	Temperature *float64

	// JSONMode 要求模型输出合法 JSON。
	// 不是所有兼容端点都支持，而且即使支持也不能信任输出 ——
	// 调用方必须再做结构与枚举校验。
	JSONMode bool

	Tools []ToolDef

	// Search 表示这次调用需要联网检索。
	// 能不能实现取决于客户端：DashScope 原生客户端支持，
	// OpenAI 兼容端点会忽略它（实测不透传 enable_search）。
	Search bool

	// Extra 是透传给服务端的额外参数，用于各家的私有开关。
	// 例如百炼的联网搜索就是 {"enable_search": true}。
	// 它只影响这一层，不改变上层的调用方式。
	Extra map[string]any
}

// Response 是模型的回复。
type Response struct {
	Content   string
	ToolCalls []ToolCall
	// Sources 是联网检索命中的网页，只有支持搜索的客户端才会返回。
	// 用它来校验模型引用的来源是否真实存在。
	Sources []Source
}

// Source 是一条检索命中的网页。
type Source struct {
	Title string
	Site  string
	URL   string
}

// Client 是模型客户端。测试和离线场景可以替换成假实现。
type Client interface {
	Chat(ctx context.Context, req Request) (Response, error)
}
