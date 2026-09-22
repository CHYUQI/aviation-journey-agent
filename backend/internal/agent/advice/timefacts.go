package advice

import (
	"strings"
	"time"

	"aviation-journey-agent/backend/internal/domain"
)

// TimeFacts 是"还来不来得及"的算术结果。
//
// 契约要求"前端只渲染、后端只算数"：时间差一律在这里算好再交给模型。
// 实测把原始时间丢给模型让它自己减会出事 —— 会出现"起飞还有 8 小时"
// 却提示"时间非常紧张"这种结论。
type TimeFacts struct {
	// ServerNow 是服务端当前时间（快照时间戳），模型判断"现在"的唯一依据。
	ServerNow string
	// MinutesUntilGateClose 是距登机口关闭还有多少分钟（没有该节点时退回起飞节点）。
	MinutesUntilGateClose *int
	// BufferMin 是到达机场时距登机口关闭还剩多少分钟：
	// 旅客已在机场时不扣路程时间；没有定位也没有手动阶段时为 nil。
	BufferMin *int
}

// ComputeTimeFacts 计算上面的三个事实。now 传服务端时钟，零值时回退到本机时间。
func ComputeTimeFacts(
	state domain.State,
	progress domain.JourneyProgress,
	etaMin *int,
	now time.Time,
) TimeFacts {
	if now.IsZero() {
		now = time.Now()
	}
	facts := TimeFacts{ServerNow: now.Format(time.RFC3339)}

	deadline, ok := gateCloseDeadline(state.Timeline)
	if !ok {
		// 连登机口关闭/起飞时间都没有，算不出任何时间差。
		return facts
	}

	minutes := int(deadline.Sub(now).Minutes())
	facts.MinutesUntilGateClose = &minutes

	switch {
	case atAirport(progress.ManualStage):
		// 人已经在机场，不需要再扣路程时间。
		buffer := minutes
		facts.BufferMin = &buffer
	case etaMin != nil:
		buffer := minutes - *etaMin
		facts.BufferMin = &buffer
	}
	return facts
}

// gateCloseDeadline 取"登机口关闭"节点的时间；没有就退回"起飞"节点。
// 两者都没有时返回 false，让调用方把时间差留空，而不是编一个。
func gateCloseDeadline(timeline []domain.TimelineNode) (time.Time, bool) {
	var departure time.Time
	hasDeparture := false

	for _, node := range timeline {
		ts, err := time.Parse(time.RFC3339, strings.TrimSpace(node.Time))
		if err != nil {
			continue
		}
		if strings.Contains(node.Label, "登机口关闭") {
			return ts, true
		}
		if strings.Contains(node.Label, "起飞") {
			departure = ts
			hasDeparture = true
		}
	}
	return departure, hasDeparture
}

// atAirport 判断旅客是否已经确认在机场（由旅客手动确认的阶段）。
func atAirport(stage string) bool {
	switch stage {
	case domain.StageAtAirport, domain.StageCheckIn, domain.StageSecurity,
		domain.StageWaiting, domain.StageBoarding:
		return true
	default:
		return false
	}
}
