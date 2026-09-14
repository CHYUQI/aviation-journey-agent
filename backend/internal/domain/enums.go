package domain

// 契约枚举值。修改这些常量属于破坏性契约变更，必须先改
// docs/api/openapi.yaml 并走 AGENTS.md 里的变更流程。

const (
	StatusProcessing = "processing"
	StatusReady      = "ready"
	StatusFailed     = "failed"
)

const (
	RiskUnknown = "unknown"
	RiskGreen   = "green"
	RiskYellow  = "yellow"
	RiskOrange  = "orange"
	RiskRed     = "red"
)

const (
	StageUnknown   = "unknown"
	StageEnRoute   = "en_route"
	StageAtAirport = "at_airport"
	StageCheckIn   = "check_in"
	StageSecurity  = "security"
	StageWaiting   = "waiting"
	StageBoarding  = "boarding"
	StageDeparted  = "departed"
	StageDisrupted = "disrupted"
)

const (
	QualityComplete = "complete"
	QualityDegraded = "degraded"
	QualityUnknown  = "unknown"
)

const (
	FlightStatusUnknown   = "unknown"
	FlightStatusScheduled = "scheduled"
	FlightStatusOnTime    = "on_time"
	FlightStatusDelayed   = "delayed"
	FlightStatusBoarding  = "boarding"
	FlightStatusDeparted  = "departed"
	FlightStatusCancelled = "cancelled"
	FlightStatusDiverted  = "diverted"
)

const (
	TrafficUnknown  = "unknown"
	TrafficLight    = "light"
	TrafficModerate = "moderate"
	TrafficHeavy    = "heavy"
	TrafficSevere   = "severe"
)

// ManualStages 是旅客可以手动确认的阶段，是全部 Stage 的子集。
// 去掉了旅客无法自行判断的 unknown / departed / disrupted。
var ManualStages = []string{
	StageEnRoute,
	StageAtAirport,
	StageCheckIn,
	StageSecurity,
	StageWaiting,
	StageBoarding,
}

// IsManualStage 判断取值是否允许旅客手动确认。
func IsManualStage(s string) bool {
	for _, v := range ManualStages {
		if v == s {
			return true
		}
	}
	return false
}

var flightStatusValues = map[string]bool{
	FlightStatusUnknown:   true,
	FlightStatusScheduled: true,
	FlightStatusOnTime:    true,
	FlightStatusDelayed:   true,
	FlightStatusBoarding:  true,
	FlightStatusDeparted:  true,
	FlightStatusCancelled: true,
	FlightStatusDiverted:  true,
}

// ValidFlightStatus 判断航班状态是否是契约里的合法取值。
// 用来挡住"上游拼错枚举"这类问题 —— 契约保证不能靠自觉。
func ValidFlightStatus(s string) bool { return flightStatusValues[s] }

var trafficValues = map[string]bool{
	TrafficUnknown:  true,
	TrafficLight:    true,
	TrafficModerate: true,
	TrafficHeavy:    true,
	TrafficSevere:   true,
}

// ValidTraffic 判断路况是否是契约里的合法取值。
func ValidTraffic(s string) bool { return trafficValues[s] }
