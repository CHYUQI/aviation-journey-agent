package advice

import (
	"strings"

	"aviation-journey-agent/backend/internal/domain"
)

// modelAdvice 是模型输出的形状。
//
// 它与 domain.Advice 有意保持接近但不完全一样：actions 里的 nav 是一个符号值
// （"airport" 表示要导航去机场），真正的跳转链接由代码生成。
// 这样模型只需要表达"要不要导航"，不用去拼一个容易写错的 URL。
type modelAdvice struct {
	Stage   string        `json:"stage"`
	Risk    string        `json:"risk"`
	Alert   *string       `json:"alert"`
	Cards   []domain.Card `json:"cards"`
	Actions []modelAction `json:"actions"`
	Reasons []string      `json:"reasons"`
}

type modelAction struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Nav    string `json:"nav"`
}

// toDomain 把模型输出转成契约结构，并做结构层面的清洗。
//
// 这里只处理"结构是否合法"，不判断业务对错 —— 业务兜底见 enforceRiskFloor。
func (m modelAdvice) toDomain(nav *domain.Nav) domain.Advice {
	result := domain.NewAdvice()
	result.Stage = m.Stage
	result.Risk = m.Risk
	result.Alert = cleanStringPtr(m.Alert)
	result.Cards = m.Cards
	result.Reasons = m.Reasons

	for _, a := range m.Actions {
		action := domain.Action{
			ID:     strings.TrimSpace(a.ID),
			Title:  strings.TrimSpace(a.Title),
			Detail: strings.TrimSpace(a.Detail),
		}
		// 只有标题和说明都有内容才算一条有效行动
		if action.ID == "" || action.Title == "" || action.Detail == "" {
			continue
		}
		// nav 由代码填，不由模型填
		if a.Nav == "airport" {
			action.Nav = nav
		}
		result.Actions = append(result.Actions, action)
	}

	// 契约要求数组不能是 null
	if result.Cards == nil {
		result.Cards = []domain.Card{}
	}
	if result.Actions == nil {
		result.Actions = []domain.Action{}
	}
	if result.Reasons == nil {
		result.Reasons = []string{}
	}

	// 过滤掉内容为空的指标卡
	validCards := make([]domain.Card, 0, len(result.Cards))
	for _, c := range result.Cards {
		if strings.TrimSpace(c.Label) != "" && strings.TrimSpace(c.Value) != "" {
			validCards = append(validCards, domain.Card{
				Label: strings.TrimSpace(c.Label),
				Value: strings.TrimSpace(c.Value),
			})
		}
	}
	result.Cards = validCards

	return result
}

// valid 检查枚举取值是否在契约白名单内。
func (m modelAdvice) valid() bool {
	return stageValues[m.Stage] && riskValues[m.Risk]
}

// CriticalDataMissing 返回判断风险所必需、但当前缺失的数据说明。
//
// 判断哪些数据算"关键"要看阶段：
//   - 还在路上时，"到机场要多久"是判断来不来得及的必要输入；
//   - 已经进了机场，"登机与起飞时间"才是。
func CriticalDataMissing(state domain.State, stage string) []string {
	missing := make([]string, 0, 3)

	if state.FlightStatus == domain.FlightStatusUnknown {
		missing = append(missing, "航班状态")
	}
	if len(state.Timeline) == 0 {
		missing = append(missing, "登机与起飞时间")
	}
	if stage == domain.StageEnRoute || stage == domain.StageUnknown {
		if state.ETAMin == nil {
			missing = append(missing, "到机场的路程时间")
		}
	}

	return missing
}

// enforceRiskFloor 是代码级的硬约束：关键数据缺失时不允许给出"安全"结论。
//
// 这条不能只写在 prompt 里 —— 模型可能忽略，也可能误判。
// 契约里写明"关键数据缺失不得返回绿色"，这里负责把它兜住。
func enforceRiskFloor(result *domain.Advice, state domain.State) {
	missing := CriticalDataMissing(state, result.Stage)
	if len(missing) == 0 {
		return
	}

	if result.Risk != domain.RiskUnknown {
		result.Reasons = append(result.Reasons,
			"缺少"+strings.Join(missing, "、")+"，已把风险降级为 unknown")
	}
	result.Risk = domain.RiskUnknown

	// 风险都判断不了，就不该再给出"立即出发"这类高强度行动
	filtered := make([]domain.Action, 0, len(result.Actions))
	for _, a := range result.Actions {
		if a.Nav != nil {
			continue
		}
		filtered = append(filtered, a)
	}
	result.Actions = filtered
}

func cleanStringPtr(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
