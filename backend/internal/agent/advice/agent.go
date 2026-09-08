package advice

import (
	"aviation-journey-agent/backend/internal/domain"
	"time"
)

type Agent struct{}

func NewAgent() *Agent { return &Agent{} }

func (a *Agent) Evaluate(state domain.WorldState) domain.Advice {
	result := domain.Advice{Stage: DetermineStage(state), RiskLevel: "unknown", Metrics: []domain.Metric{}, Actions: []domain.Action{}, Reasoning: []string{}}
	if state.DataQuality.Status != "complete" {
		result.Reasoning = append(result.Reasoning, "关键数据尚未完整获取，当前建议仅供参考")
	}
	if !state.Derived.LatestDeparture.IsZero() {
		result.Metrics = append(result.Metrics, domain.Metric{Key: "latest_departure", Label: "最晚出发", Value: state.Derived.LatestDeparture.Format(time.RFC3339), Unit: "time"})
	}
	if state.Travel.ETAMin > 0 {
		result.Metrics = append(result.Metrics, domain.Metric{Key: "eta_min", Label: "预计到达机场", Value: state.Travel.ETAMin, Unit: "min"})
	}
	if state.Derived.BufferMin != 0 {
		result.Metrics = append(result.Metrics, domain.Metric{Key: "buffer_min", Label: "缓冲时间", Value: state.Derived.BufferMin, Unit: "min"})
	}
	// TODO: 按硬约束、阶段和风险规则生成行动；不在此处执行外部副作用。
	return result
}
