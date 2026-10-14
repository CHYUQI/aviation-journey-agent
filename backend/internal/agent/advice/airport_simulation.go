package advice

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"aviation-journey-agent/backend/internal/domain"
)

// 这些是流程模拟的假设，不是机场实测数据；只写入行动说明及判断依据，
// 不写入 State、指标卡、提醒或风险计算。
const (
	simulatedWalkBaseMin     = 10
	simulatedCheckInBaseMin  = 15
	simulatedBaggageExtraMin = 10
	simulatedSecurityBaseMin = 20
	airportSimulationMarker  = "\n\n机场流程模拟估算（非实测）："
	airportSimulationReason  = "机场流程模拟依据："
)

// 只消费 AirportStatusSkill 已核实并写入 State.Guide 的出发计划客流。
// 到达客流、延误统计、缺失时段均不能替代当前小时出发计划座位数。
var departureTrafficPattern = regexp.MustCompile(`^客流（出发）：当前小时计划旅客座位数 ([0-9]+) 座，未来 24 小时合计约 ([0-9]+) 座$`)

type airportTimeSimulation struct {
	walkMin     int
	checkInMin  int
	securityMin int
	source      string
}

func simulateAirportTimes(guide []string, hasBaggage bool) (airportTimeSimulation, bool) {
	for _, line := range guide {
		line = strings.TrimSpace(line)
		matches := departureTrafficPattern.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		current, currentErr := strconv.Atoi(matches[1])
		total, totalErr := strconv.Atoi(matches[2])
		if currentErr != nil || totalErr != nil || current <= 0 || total < current {
			return airportTimeSimulation{}, false
		}

		// 用当前小时 / 未来 24 小时的小时均值表示相对负荷。
		// 倍率限制在 1~2，低客流也不缩短基准预留；整数分钟向上取整。
		factor := math.Max(1, math.Min(2, float64(current)*24/float64(total)))
		checkInBase := simulatedCheckInBaseMin
		if hasBaggage {
			checkInBase += simulatedBaggageExtraMin
		}
		return airportTimeSimulation{
			walkMin:     int(math.Ceil(simulatedWalkBaseMin * factor)),
			checkInMin:  int(math.Ceil(float64(checkInBase) * factor)),
			securityMin: int(math.Ceil(simulatedSecurityBaseMin * factor)),
			source:      line,
		}, true
	}
	return airportTimeSimulation{}, false
}

// WithAirportTimeSimulation 更新已有行动的流程模拟文案，不新增行动或卡片。
// 刷新复用旧建议时也调用它，防止继续显示旧客流或已完成阶段的模拟。
// 复制切片并先清除上次模拟，避免修改缓存快照或重复追加文案。
func WithAirportTimeSimulation(result domain.Advice, in Input) domain.Advice {
	result.Actions = append([]domain.Action{}, result.Actions...)
	for i := range result.Actions {
		result.Actions[i].Detail, _, _ = strings.Cut(result.Actions[i].Detail, airportSimulationMarker)
	}
	reasons := make([]string, 0, len(result.Reasons)+1)
	for _, reason := range result.Reasons {
		if !strings.HasPrefix(reason, airportSimulationReason) {
			reasons = append(reasons, reason)
		}
	}
	result.Reasons = reasons

	if len(result.Actions) == 0 || RiskConstraintFor(in.State, result.Stage).Unknown {
		return result
	}
	if in.State.FlightStatus == domain.FlightStatusDeparted || in.State.FlightStatus == domain.FlightStatusCancelled {
		return result
	}
	switch result.Stage {
	case domain.StageBoarding, domain.StageDeparted, domain.StageDisrupted:
		return result
	}

	index := airportSimulationAction(result.Actions, result.Stage)
	if index < 0 {
		return result
	}
	simulation, ok := simulateAirportTimes(in.State.Guide, in.HasBaggage)
	if !ok {
		result.Reasons = append(result.Reasons, airportSimulationReason+
			"缺少可用的 EOOB 当前小时出发计划客流及未来 24 小时统计，暂不提供机场流程时长模拟")
		return result
	}

	walk := fmt.Sprintf("步行时间（模拟）：约 %d 分钟（机场内通用动线）", simulation.walkMin)
	if result.Stage == domain.StageWaiting {
		walk += "，仅在需要前往登机口时预留"
	}
	steps := []string{walk}
	if result.Stage != domain.StageSecurity && result.Stage != domain.StageWaiting {
		if in.HasBaggage {
			steps = append(steps, fmt.Sprintf("值机时间（模拟）：约 %d 分钟（含托运，完成后再安检）", simulation.checkInMin))
		} else {
			steps = append(steps, fmt.Sprintf("值机时间（模拟）：约 %d 分钟（仅需柜台值机时预留，已完成可跳过）", simulation.checkInMin))
		}
	}
	if result.Stage != domain.StageWaiting {
		steps = append(steps, fmt.Sprintf("安检时间（模拟）：约 %d 分钟（含排队及检查）", simulation.securityMin))
	}
	result.Actions[index].Detail += airportSimulationMarker + "\n" + strings.Join(steps, "\n") + "\n仅供时间预留，以现场情况为准。"

	reason := airportSimulationReason + "来源 EOOB 机场页；" + simulation.source +
		fmt.Sprintf("。模拟基准：步行 %d 分钟、柜台值机 %d 分钟（有托运另加 %d 分钟）、安检 %d 分钟；",
			simulatedWalkBaseMin, simulatedCheckInBaseMin, simulatedBaggageExtraMin, simulatedSecurityBaseMin) +
		"乘以当前小时计划客流相对未来 24 小时小时均值的倍率（限制在 1~2 倍，向上取整）。" +
		"缺少实际动线、开放柜台及安检通道数量和实测排队数据，不代表真实耗时，未用于风险或缓冲计算。"
	if in.State.UpdatedAt != "" {
		reason += "快照生成于 " + in.State.UpdatedAt
	}
	result.Reasons = append(result.Reasons, reason)
	return result
}

func airportSimulationAction(actions []domain.Action, stage string) int {
	keywords := []string{"机场", "值机", "托运", "安检", "步行", "登机口"}
	if stage == domain.StageSecurity {
		keywords = []string{"安检", "步行", "登机口"}
	} else if stage == domain.StageWaiting {
		keywords = []string{"步行", "登机口"}
	}
	// 优先匹配行动标题，避免“开启定位”说明中的“到机场”抢走流程提示。
	for pass := 0; pass < 2; pass++ {
		for i, action := range actions {
			if strings.Contains(action.Title, "定位") || strings.Contains(action.Title, "位置") {
				continue
			}
			if action.Nav != nil && (stage == domain.StageUnknown || stage == domain.StageEnRoute) {
				return i
			}
			text := action.Title
			if pass == 1 {
				text += " " + action.Detail
			}
			for _, keyword := range keywords {
				if strings.Contains(text, keyword) {
					return i
				}
			}
		}
	}
	return -1
}
