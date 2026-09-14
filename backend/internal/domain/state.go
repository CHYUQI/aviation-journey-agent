package domain

// State 是 DataAgent 的输出。对应契约 schemas.State。
//
// 硬性约定：
//   - 只包含有依据的数据。没有依据的字段一律为 nil，禁止用默认值填充。
//   - 数组无内容时返回空切片，序列化后是 []，不是 null。
//   - 时间字段是 RFC3339 带时区字符串，时长是整数分钟。
//   - 所有可空字段都不加 omitempty，未知值要以 null 的形式出现在 JSON 里。
type State struct {
	FlightStatus string         `json:"flightStatus"`
	Gate         *string        `json:"gate"`
	Timeline     []TimelineNode `json:"timeline"`
	ETAMin       *int           `json:"etaMin"`
	Traffic      *string        `json:"traffic"`
	Guide        []string       `json:"guide"`
	Quality      string         `json:"quality"`
	UpdatedAt    string         `json:"updatedAt"`
}

// TimelineNode 是时间轴上的一个节点，按时间升序排列。
// Time 用 RFC3339 而不是格式化后的 "14:20"，因为前端要用它做倒计时。
type TimelineNode struct {
	Label string `json:"label"`
	Time  string `json:"time"`
}

// NewState 返回一份"全部未知"的状态，作为分析起点。
// 可空字段保持 nil，这样序列化时输出 null 而不是被省略。
func NewState() State {
	return State{
		FlightStatus: FlightStatusUnknown,
		Timeline:     []TimelineNode{},
		Guide:        []string{},
		Quality:      QualityUnknown,
	}
}
