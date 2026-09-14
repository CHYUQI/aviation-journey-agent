package skill

import (
	"context"

	"aviation-journey-agent/backend/internal/domain"
)

// Query 是一次数据查询的输入。
type Query struct {
	Journey domain.Journey
	// Location 为 nil 表示还没拿到定位，需要位置的 Skill 应当直接返回空结果
	Location *domain.Coordinate
	// Fields 是本次要查的字段，取值见 fields.go
	Fields []string
}

// Observation 是 Skill 返回的一条观测值。
//
// Source / ObservedAt / Confidence 是数据可信度的依据：
// 没有依据时应当不返回 Observation，而不是返回一个猜测值。
type Observation struct {
	Field      string
	Value      any
	Source     string
	ObservedAt string // RFC3339
	Confidence string // high / medium / low
}

// Result 是一次查询的结果。
type Result struct {
	Observations []Observation
	Issues       []string
}

// Skill 是一个可被 DataAgent 调用的数据能力。
//
// Skill 只负责"取一条有依据的数据"，不做判断、不拼建议。
type Skill interface {
	Name() string
	Supports(field string) bool
	Execute(ctx context.Context, query Query) (Result, error)
}
