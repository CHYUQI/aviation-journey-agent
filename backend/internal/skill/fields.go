package skill

// DataAgent 查询的字段名。
//
// 各 Skill 的 Supports 与 data.MissingFields 必须都使用这些常量，
// 避免两边字符串写错导致"没有技能可以查询"的静默失败。
const (
	FieldFlightStatus  = "flight.status"
	FieldFlightTimes   = "flight.times"
	FieldFlightGate    = "flight.gate"
	FieldTravelETA     = "travel.eta"
	FieldAirportGuide  = "airport.guide"
	FieldOfficialNotes = "official.notes"
)
