package data

import "aviation-journey-agent/backend/internal/domain"

// ApplyDerivedFields 只放可解释的确定性派生计算。
func ApplyDerivedFields(state *domain.WorldState) {
	// TODO: 使用真实观测值和规则配置推导 latestDeparture、bufferMin 等字段。
}
