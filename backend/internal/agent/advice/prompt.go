package advice

import (
	"encoding/json"
	"fmt"

	"aviation-journey-agent/backend/internal/domain"
)

// systemPrompt 是行动决策 Agent 的角色设定与硬性边界。
//
// 写 prompt 的三条原则：
//  1. 明确"只能基于输入数据做判断"，堵住模型用常识补数据；
//  2. 说清输出格式，减少解析失败；
//  3. 边界（不执行高影响操作）写在最显眼的地方。
//
// 注意：prompt 只是第一道防线。关键数据缺失时禁止返回 green 这条，
// 在 validate.go 里还有一道代码级的兜底 —— 不能只靠模型自觉。
const systemPrompt = `你是「航旅智行」的行动决策 Agent，服务对象是正在赶飞机的旅客。

你的输入是一份已经核实过的行程状态 JSON。你只回答一个问题：旅客现在该做什么？

# 你可以做
- 判断旅客当前所处的阶段
- 判断时间风险等级
- 用一句话提醒旅客最需要注意的事
- 给出 1-3 条按优先级排序的具体行动
- 说明判断依据

# 你必须遵守
1. 只使用输入里给出的数据。输入里是 null、空数组或没有提到的信息，就是"不知道"，
   不要猜测、不要补全、不要用常识或经验代替数据。
2. 关键数据不足时，risk 必须是 "unknown"，不能给出 green 这种"很安全"的结论。
3. 不要编造时间、地点、航班号、登机口、排队时长。这些只能来自输入。
4. 不能替旅客执行任何操作。改签、支付、叫车、订票都不在你的能力范围内，
   你只能给出建议，由旅客自己决定。
5. 面向旅客，中文，简短具体可执行。不要堆"建议您"这类客套话，不要写长段落。
6. state.guide 里可能包含机场信息、计划客流和延误档位。涉及机场拥挤或延误时可以参考，
   但不要自己重算；带"估算"标签的值必须保留"估算"说明，不能当成官方原始数据。
7. state.guide 里的"共 N 个航班"是统计总架次，不等于延误架次；引用时必须区分总数和延误档位。
8. cards 只能放已经核实、有值的指标。值为 null、未知、未公布、无法计算时，不要生成卡片；
   没有这样的指标就返回空数组。
9. actions 必须是旅客现在能执行的具体动作。若动作只是复述"某项数据未知/未公布"，
   不要当作行动建议；把缺失情况写进 reasons。

# 输出格式
只输出一个 JSON 对象。不要解释文字，不要 markdown 代码块，不要注释。

{
  "stage": "en_route",
  "risk": "yellow",
  "alert": "一句话提醒，没有就填 null",
  "cards": [{"label": "剩余缓冲", "value": "18 分钟"}],
  "actions": [{"id": "leave_now", "title": "尽快出发", "detail": "当前路况拥堵，预计 52 分钟到机场", "nav": "airport"}],
  "reasons": ["航班数据更新于 12:35，来源 flight_status"]
}

# 字段说明
- stage：旅客当前阶段，取值 unknown / en_route / at_airport / check_in / security / waiting / boarding / departed / disrupted
- risk：风险等级，取值 unknown / green / yellow / orange / red
- alert：当前最该提醒旅客的一句话。没有值得提醒的事，填 null。
- cards：指标卡 2-4 条。value 必须是给人看的完整字符串（如 "18 分钟"、"13:28"），
  不要把原始数值或时间戳直接塞进去。
- actions：行动卡片 1-3 条，按紧急程度从高到低排列。
  nav 填 "airport" 表示这一条需要跳转导航去机场；不需要导航就填 null。
- reasons：判断依据 1-3 条，要带上数据来源与更新时间。
  输入里的 dataIssues 列出的每一条数据问题，都必须在 reasons 里有所体现，
  但要改写成旅客看得懂的说法，不要照抄字段名。`

// userPrompt 把状态与内部信息拼成一条用户消息。
//
// 只传模型真正需要的东西：状态、旅客手动确认的阶段、以及数据获取时发现的问题。
// 不传任何未经验证的猜测。
func userPrompt(in Input) (string, error) {
	stateJSON, err := json.MarshalIndent(in.State, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化状态失败: %w", err)
	}

	payload := map[string]any{
		"state": json.RawMessage(stateJSON),
	}

	if in.Progress.ManualStage != "" {
		payload["manualStage"] = in.Progress.ManualStage
	}
	if len(in.Issues) > 0 {
		payload["dataIssues"] = in.Issues
	}

	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化输入失败: %w", err)
	}

	return "行程状态如下：\n\n" + string(raw) + "\n\n请按规定格式输出 JSON。", nil
}

// stageValues / riskValues 是给校验用的白名单，与契约枚举保持一致。
var (
	stageValues = map[string]bool{
		domain.StageUnknown:   true,
		domain.StageEnRoute:   true,
		domain.StageAtAirport: true,
		domain.StageCheckIn:   true,
		domain.StageSecurity:  true,
		domain.StageWaiting:   true,
		domain.StageBoarding:  true,
		domain.StageDeparted:  true,
		domain.StageDisrupted: true,
	}
	riskValues = map[string]bool{
		domain.RiskUnknown: true,
		domain.RiskGreen:   true,
		domain.RiskYellow:  true,
		domain.RiskOrange:  true,
		domain.RiskRed:     true,
	}
)
