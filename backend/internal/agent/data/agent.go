package data

import (
	"context"
	"log"
	"sort"
	"strings"
	"time"

	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/skill"
)

// Input 是 DataAgent 的输入。
type Input struct {
	Journey  domain.Journey
	Progress domain.JourneyProgress
	// Previous 是上一版状态，用于判断哪些字段还需要刷新
	Previous *domain.State
}

// Result 是 DataAgent 的输出。
//
// Issues 记录取数过程中的问题，最终会进入 advice.reasons 展示给旅客。
type Result struct {
	State  domain.State
	Issues []string
}

type Agent struct{ skills *skill.Registry }

func NewAgent(skills *skill.Registry) *Agent { return &Agent{skills: skills} }

// BuildState 组装行程状态。
//
// 流程：找出缺哪些字段 -> 交给技能去取 -> 把观测值标准化进 State -> 评估数据完整度。
//
// 硬性约束：没有观测值的字段一律保持 null，绝不用默认值填充。
// 取数失败时状态里就是 null，由 AdviceAgent 走降级路径。
func (a *Agent) BuildState(ctx context.Context, in Input) (Result, error) {
	state := domain.NewState()
	result := Result{State: state, Issues: []string{}}

	if a.skills == nil {
		result.Issues = append(result.Issues, "数据技能未注册，暂时无法获取航班与机场数据")
		return result, nil
	}

	fields := MissingFields(state)
	if len(fields) == 0 {
		state.Quality = domain.QualityComplete
		result.State = state
		return result, nil
	}

	queryResult, err := a.skills.Query(ctx, skill.Query{
		Journey:  in.Journey,
		Location: in.Progress.Location,
		Fields:   fields,
	})
	if err != nil {
		result.Issues = append(result.Issues, err.Error())
	}

	result.Issues = append(result.Issues, queryResult.Issues...)
	// 取数问题同时打到服务端日志：进 reasons 的那份会被模型改写成旅客用语，
	// 排查时看不出原始原因。
	for _, issue := range result.Issues {
		log.Printf("[data] journey=%s %s", in.Journey.ID, issue)
	}
	applyObservations(&state, queryResult.Observations)
	state.Quality = assessQuality(state)

	result.State = state
	return result, nil
}

// applyObservations 把技能返回的观测值写进状态。
//
// 每个字段都做一次类型与取值校验：技能出错时宁可字段留 null，
// 也不能把非法枚举或格式错误的时间写进对外契约。
func applyObservations(state *domain.State, observations []skill.Observation) {
	for _, obs := range observations {
		switch obs.Field {
		case skill.FieldFlightStatus:
			value, ok := obs.Value.(string)
			if ok && domain.ValidFlightStatus(value) {
				state.FlightStatus = value
			}

		case skill.FieldFlightGate:
			value, ok := obs.Value.(string)
			value = strings.TrimSpace(value)
			if ok && value != "" {
				gate := value
				state.Gate = &gate
			}

		case skill.FieldFlightTimes:
			if times, ok := obs.Value.(skill.FlightTimes); ok {
				state.Timeline = buildTimeline(times)
			}

		case skill.FieldAirportGuide:
			if guide, ok := obs.Value.([]string); ok {
				state.Guide = nonEmptyLines(guide)
			}
		}
	}
}

// buildTimeline 把航班时间整理成契约要求的时间轴节点。
//
// 时间是模型从网页里读出来的，格式不一定可靠：解析不过的一律丢掉。
// 节点按时间升序排列，因为契约要求如此。
func buildTimeline(times skill.FlightTimes) []domain.TimelineNode {
	nodes := make([]domain.TimelineNode, 0, 3)

	add := func(label, raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if _, err := time.Parse(time.RFC3339, raw); err != nil {
			return
		}
		nodes = append(nodes, domain.TimelineNode{Label: label, Time: raw})
	}

	add("开始登机", times.BoardingTime)
	add("登机口关闭", times.GateCloseTime)
	add("起飞", times.DepartureTime)

	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Time < nodes[j].Time })
	return nodes
}

func nonEmptyLines(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// assessQuality 评估数据完整度。
//
// 判据是"能不能支撑一次有意义的判断"：
//   - 航班状态未知、时间轴为空 -> unknown，什么都判断不了
//   - 两者都有 -> complete
//   - 只有一部分 -> degraded
//
// 路程时间不参与判定：旅客可能已经在机场，那时它本来就不需要。
func assessQuality(state domain.State) string {
	hasFlight := state.FlightStatus != domain.FlightStatusUnknown
	hasTimeline := len(state.Timeline) > 0

	switch {
	case hasFlight && hasTimeline:
		return domain.QualityComplete
	case hasFlight || hasTimeline:
		return domain.QualityDegraded
	default:
		return domain.QualityUnknown
	}
}
