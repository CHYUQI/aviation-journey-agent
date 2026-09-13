package data

import (
	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/skill"
)

// MissingFields 返回当前状态里还没有依据、需要向上游查询的字段。
//
// 字段名必须与各 Skill 的 Supports 保持一致（见 skill/fields.go）。
func MissingFields(state domain.State) []string {
	fields := make([]string, 0, 5)

	if state.FlightStatus == domain.FlightStatusUnknown {
		fields = append(fields, skill.FieldFlightStatus)
	}
	if state.Gate == nil {
		fields = append(fields, skill.FieldFlightGate)
	}
	if len(state.Timeline) == 0 {
		fields = append(fields, skill.FieldFlightTimes)
	}
	if state.ETAMin == nil {
		fields = append(fields, skill.FieldTravelETA)
	}
	if len(state.Guide) == 0 {
		fields = append(fields, skill.FieldAirportGuide)
	}

	return fields
}
