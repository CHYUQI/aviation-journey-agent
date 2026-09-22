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

	// 过滤掉内容为空或值为"未知"的指标卡。
	// 未知信息应进 reasons，不应作为面向旅客的数字卡片。
	validCards := make([]domain.Card, 0, len(result.Cards))
	for _, c := range result.Cards {
		label := strings.TrimSpace(c.Label)
		value := strings.TrimSpace(c.Value)
		if label == "" || value == "" || isUnknownText(value) {
			continue
		}
		validCards = append(validCards, domain.Card{Label: label, Value: value})
	}
	result.Cards = validCards

	return result
}

// valid 检查枚举取值是否在契约白名单内。
func (m modelAdvice) valid() bool {
	return stageValues[m.Stage] && riskValues[m.Risk]
}

// RiskConstraint 描述"关键数据缺失"对风险等级的约束。
//
// 分两级，避免把"缺一点"和"什么都不知道"混为一谈：
//   - Unknown = false：还能做有限判断，只要求不得给 green（最高 yellow）；
//   - Unknown = true ：连航班状态和起飞时间都没有，完全判断不了，只能 unknown。
type RiskConstraint struct {
	Missing []string
	Unknown bool
}

// RiskConstraintFor 计算当前状态对风险等级的约束。
//
// 判断哪些数据算"关键"要看阶段：还在路上时"到机场要多久"是必要输入；
// 已经进了机场时，"登机与起飞时间"才是。
func RiskConstraintFor(state domain.State, stage string) RiskConstraint {
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

	if len(missing) == 0 {
		return RiskConstraint{}
	}

	// 航班状态和起飞时间都没有：没有任何判断依据，只能 unknown。
	noFlightStatus := state.FlightStatus == domain.FlightStatusUnknown
	noTimeline := len(state.Timeline) == 0
	return RiskConstraint{Missing: missing, Unknown: noFlightStatus && noTimeline}
}

// enforceRiskFloor 是代码级的硬约束：关键数据缺失时不允许给出"安全"结论。
//
// 契约的原话是"关键数据缺失时 risk 必须为 unknown 或至少 yellow"——
// 所以缺失不等于"必须 unknown"：
//   - 还能判断（知道航班状态、知道起飞时间，只是缺路况/登机口）→ 压到 yellow；
//   - 完全判断不了（连航班状态和时间都没有）→ unknown，并撤掉高强度行动。
//
// 这条不能只写在 prompt 里 —— 模型可能忽略，也可能误判。
func enforceRiskFloor(result *domain.Advice, state domain.State) {
	constraint := RiskConstraintFor(state, result.Stage)
	if len(constraint.Missing) == 0 {
		return
	}

	if !constraint.Unknown {
		if result.Risk == domain.RiskGreen {
			result.Risk = domain.RiskYellow
			result.Reasons = append(result.Reasons,
				"缺少"+strings.Join(constraint.Missing, "、")+"，不能判定为安全，风险降级为 yellow")
		}
		return
	}

	if result.Risk != domain.RiskUnknown {
		result.Reasons = append(result.Reasons,
			"缺少"+strings.Join(constraint.Missing, "、")+"，无法判断风险，已降级为 unknown")
	}
	result.Risk = domain.RiskUnknown

	// 风险都判断不了，就不该再给出"立即出发"这类高强度行动；
	// 只复述"未知/未公布/无法计算"的行动也不输出，避免把缺失数据包装成建议。
	filtered := make([]domain.Action, 0, len(result.Actions))
	for _, a := range result.Actions {
		if a.Nav != nil || actionRestatesMissing(a) {
			continue
		}
		filtered = append(filtered, a)
	}
	result.Actions = filtered
}

func isUnknownText(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return true
	}
	for _, marker := range []string{
		"未知", "未公布", "尚未公布", "待公布", "暂无", "无数据", "没有数据",
		"无法计算", "无法判断", "unknown", "n/a", "--",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func actionRestatesMissing(action domain.Action) bool {
	return isUnknownText(action.Title) || isUnknownText(action.Detail)
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
