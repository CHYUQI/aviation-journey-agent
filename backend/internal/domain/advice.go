package domain

// Advice 是 AdviceAgent 的输出。对应契约 schemas.Advice。
//
// 约定：Stage、Risk 必须始终是合法枚举值；数据不足时 Risk 用 unknown，
// 绝不允许在没有依据的情况下返回 green。
type Advice struct {
	Stage   string   `json:"stage"`
	Risk    string   `json:"risk"`
	Alert   *string  `json:"alert"`
	Cards   []Card   `json:"cards"`
	Actions []Action `json:"actions"`
	Reasons []string `json:"reasons"`
}

// Card 是指标卡。Value 是已经格式化好的展示字符串（如 "18 分钟"、"13:28"），
// 前端直接显示，不做任何计算。
type Card struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Action 是行动卡片。Nav 有值时前端才渲染导航按钮。
// 它只是给前端展示的动作，后端不执行任何高影响操作。
type Action struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Nav    *Nav   `json:"nav"`
}

// Nav 是导航跳转地址。App 优先唤起，失败时回退 Web。
type Nav struct {
	App string `json:"app"`
	Web string `json:"web"`
}

// JourneySnapshot 是前端唯一数据源。对应契约 schemas.JourneySnapshot。
//
// 所有字段始终输出；Error 例外，它只在 Status = failed 时出现，
// 在契约里也不是必填字段，因此保留 omitempty。
type JourneySnapshot struct {
	Status  string  `json:"status"`
	Journey Journey `json:"journey"`
	State   State   `json:"state"`
	Advice  Advice  `json:"advice"`
	Error   *string `json:"error,omitempty"`
}

// NewAdvice 返回一份"暂无建议"的建议，作为分析起点。
func NewAdvice() Advice {
	return Advice{
		Stage:   StageUnknown,
		Risk:    RiskUnknown,
		Cards:   []Card{},
		Actions: []Action{},
		Reasons: []string{},
	}
}
