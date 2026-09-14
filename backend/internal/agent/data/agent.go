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
}

// Result 是 DataAgent 的输出。
//
// Issues 记录取数过程中的问题，最终会进入 advice.reasons 展示给旅客。
type Result struct {
	State domain.State
	// Journey 是补全最近班次和航线后的行程。
	// DataAgent 只补内部缺失字段，不修改对外契约。
	Journey domain.Journey
	Issues  []string
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
	journey, resolveIssues := a.resolveJourneyIdentity(ctx, in.Journey)
	result := Result{State: state, Journey: journey, Issues: resolveIssues}

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
		Journey:  journey,
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
		log.Printf("[data] journey=%s %s", journey.ID, issue)
	}
	applyObservations(&state, queryResult.Observations)
	state.Quality = assessQuality(state, etaRequired(in.Progress))

	result.State = state
	return result, nil
}

// resolveJourneyIdentity 在行程缺日期或航线时，用 EOOB 航班号搜索接口查询，
// 解析最近班次并补全 Journey.Flights[0]。
//
// 如果调用方已经提供了完整行程，就不查询；对外契约保持不变。
func (a *Agent) resolveJourneyIdentity(ctx context.Context, journey domain.Journey) (domain.Journey, []string) {
	issues := []string{}
	if a.skills == nil || len(journey.Flights) == 0 {
		return journey, issues
	}

	flight := &journey.Flights[0]
	if strings.TrimSpace(flight.Number) == "" {
		return journey, issues
	}
	if strings.TrimSpace(flight.Date) != "" && strings.TrimSpace(flight.From) != "" && strings.TrimSpace(flight.To) != "" {
		return journey, issues
	}

	queryResult, err := a.skills.Query(ctx, skill.Query{
		Journey: journey,
		Fields:  []string{skill.FieldFlightIdentity},
	})
	if err != nil {
		issues = append(issues, err.Error())
	}
	issues = append(issues, queryResult.Issues...)

	for _, obs := range queryResult.Observations {
		identity, ok := obs.Value.(skill.ResolvedFlightIdentity)
		if !ok {
			continue
		}
		if identity.Number != "" {
			flight.Number = identity.Number
		}
		if identity.Date != "" {
			flight.Date = identity.Date
		}
		if identity.From != "" {
			flight.From = identity.From
		}
		if identity.To != "" {
			flight.To = identity.To
		}
	}

	return journey, issues
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

		case skill.FieldTravelETA:
			switch value := obs.Value.(type) {
			case int:
				etaMin := value
				state.ETAMin = &etaMin
			case *int:
				if value != nil {
					etaMin := *value
					state.ETAMin = &etaMin
				}
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
// complete 要求：航班状态、时间轴、机场信息、登机口都有依据；
// 如果旅客还没到机场，还必须有到机场的路程时间。
//
// 缺任何一项都会降为 degraded；航班状态和时间轴都没有时是 unknown。
// 不编造数据，也不把"未知"算成完整。
func assessQuality(state domain.State, etaRequired bool) string {
	hasFlight := state.FlightStatus != domain.FlightStatusUnknown
	hasTimeline := len(state.Timeline) > 0

	if !hasFlight && !hasTimeline {
		return domain.QualityUnknown
	}
	if !hasFlight || !hasTimeline {
		return domain.QualityDegraded
	}
	if len(state.Guide) == 0 {
		return domain.QualityDegraded
	}
	if state.Gate == nil {
		return domain.QualityDegraded
	}
	if etaRequired && state.ETAMin == nil {
		return domain.QualityDegraded
	}
	return domain.QualityComplete
}

// etaRequired 判断当前阶段是否必须知道"到机场要多久"。
// 已经在机场或更后面的阶段时，ETA 不再是关键数据。
func etaRequired(progress domain.JourneyProgress) bool {
	switch progress.ManualStage {
	case domain.StageAtAirport, domain.StageCheckIn, domain.StageSecurity,
		domain.StageWaiting, domain.StageBoarding:
		return false
	default:
		return true
	}
}
