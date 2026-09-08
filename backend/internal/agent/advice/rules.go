package advice

import "aviation-journey-agent/backend/internal/domain"

func DetermineStage(state domain.WorldState) string {
	if state.Flight.Status == "cancelled" {
		return "disrupted"
	}
	if state.Passenger.Location == nil {
		return "unknown"
	}
	return "en_route"
}

func RiskLevel(bufferMin int, dataComplete bool) string {
	if !dataComplete {
		return "unknown"
	}
	if bufferMin < 0 {
		return "red"
	}
	if bufferMin < 30 {
		return "yellow"
	}
	return "green"
}

func EvaluateHardConstraints(state domain.WorldState) []string {
	issues := make([]string, 0)
	if state.Flight.GateCloseTime.IsZero() {
		issues = append(issues, "gate close time is unknown")
	}
	return issues
}
