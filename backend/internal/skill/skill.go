package skill

import (
	"aviation-journey-agent/backend/internal/domain"
	"context"
)

type Query struct {
	Journey  domain.Journey
	Previous *domain.WorldState
	Fields   []string
}
type Observation struct {
	Field      string
	Value      interface{}
	Source     string
	ObservedAt string
	Confidence string
	Method     string
}
type Result struct {
	Observations []Observation
	Issues       []string
}
type Skill interface {
	Name() string
	Supports(field string) bool
	Execute(context.Context, Query) (Result, error)
}
