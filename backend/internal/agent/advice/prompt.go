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
2. 时间够不够看 bufferMin（后端已经算好），不要凭感觉：
   bufferMin 充足（>= 90 分钟）时，不要因为"路况未知 / 登机口未公布"这类精度不足就给 yellow，
   把缺失写进 reasons 即可；bufferMin 为 null 或很小时才不允许给 green。
   连航班状态和起飞时间都没有、完全没有判断依据时，才给 unknown。
3. 不要编造时间、地点、航班号、登机口、排队时长。这些只能来自输入。
4. 不能替旅客执行任何操作。改签、支付、叫车、订票都不在你的能力范围内，
   你只能给出建议，由旅客自己决定。
5. 面向旅客，中文，简短具体可执行。不要堆"建议您"这类客套话，不要写长段落。
6. state.guide 里可能包含机场信息、计划客流和延误档位。涉及机场拥挤或延误时可以参考，
   但不要自己重算；带"估算"标签的值必须保留"估算"说明，不能当成官方原始数据。
7. state.guide 里的"共 N 个航班"是统计总架次，不等于延误架次；引用时必须区分总数和延误档位。
8. cards 只能放已经核实、有值的指标。值为 null、未知、未公布、无法计算时，不要生成卡片。
   反过来：输入里已经核实且有值的指标（计划起飞时间、航班状态、登机口、预计到达时长、
   剩余缓冲等）必须出卡，不要因为"整体数据不完整"就一张都不给。
   例：etaMin=72 时必须给出「预计到达机场：72 分钟」；没有可核实的指标才返回空数组。
9. actions 必须是旅客现在能执行的具体动作。若动作只是复述"某项数据未知/未公布"，
   不要当作行动建议；把缺失情况写进 reasons。
10. 旅客所处阶段只能来自输入数据：
    - locationProvided 为 true 时，可以据此判断 en_route / at_airport 等与位置相关的阶段；
    - 没有 locationProvided、也没有 manualStage 时，stage 一律填 unknown，
      不要凭"航班快起飞了""现在该在路上了"之类的常识推断；
    - 唯一例外：航班状态已经是 departed / cancelled 时，可以填 departed / disrupted。
11. 航班状态已经是 departed 或 cancelled 时，不要再给"到机场耗时""赶路"这类卡片和提醒，
    改为核对行程或关注后续安排。
12. 输入里的 serverNow 就是当前时间，可以拿它和 timeline / bufferMin 判断时间是否充裕；
    但不要推算"数据过期了多久"，只按 state.updatedAt 的原值说明更新时间。
13. 托运行李会改变行动顺序，必须体现：hasBaggage=true 时，行动或提醒里要点出先去值机柜台
    托运行李再安检（时间紧时优先提醒这一环）；hasBaggage=false 时不要凭空加托运步骤。
14. risk 按 bufferMin（到达机场时距登机口关闭还剩多少分钟）判档：
    bufferMin >= 90 且航班正常 → green；30~90 → yellow；0~30 → orange；< 0 → red。
    bufferMin 为 null（例如没有定位）时不得给 green；航班已起飞/取消按规则 11 处理。
    minutesUntilGateClose 是"从现在到登机口关闭"的总时长、不含路程；
    没有 bufferMin 时不要把它写成"剩余缓冲"，要写成"距登机口关闭 X 分钟"。

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
- stage：旅客当前阶段，取值 unknown / en_route / at_airport / check_in / security / waiting / boarding / departed / disrupted。
  locationProvided=true 表示旅客已上报坐标，可据此判断是否在路上；没有它也没有 manualStage 时必须填 unknown。
- risk：风险等级，取值 unknown / green / yellow / orange / red
- alert：当前最该提醒旅客的一句话。没有值得提醒的事，填 null。
- cards：指标卡 1-4 条，按旅客关心程度排序。**一张卡只放一个指标**，
  value 是简短展示字符串且必须带单位（如 "18 分钟"、"13:28"、"5338 座"）；
  不要把两个数字塞进同一张卡，源数据里的"计划 / 估算 / 暂缺"等限定词要保留。
  统计类信息只取最关键的一个数字，例如「机场延误」填 "11 班延误≥15 分钟"，
  不要把整段统计原文搬进卡片。
  已核实且有值的指标必须出卡；确实一条都没有时才返回空数组。
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
		// 行李是有无托运的行程事实，直接影响"要不要先去值机柜台"。
		"hasBaggage": in.HasBaggage,
	}

	// 时间是判断"来不来得及"的必要输入：serverNow 是服务端当前时刻，
	// minutesUntilGateClose / bufferMin 由代码算好，模型只做判断不做减法。
	facts := ComputeTimeFacts(in.State, in.Progress, in.State.ETAMin, in.Now)
	payload["serverNow"] = facts.ServerNow
	if facts.MinutesUntilGateClose != nil {
		payload["minutesUntilGateClose"] = *facts.MinutesUntilGateClose
	}
	if facts.BufferMin != nil {
		payload["bufferMin"] = *facts.BufferMin
	}

	if in.Progress.ManualStage != "" {
		payload["manualStage"] = in.Progress.ManualStage
	}
	// 只告诉模型"有没有定位"，不下发具体坐标：
	// 阶段判断需要它，但精确位置属于旅客隐私，与决策无关。
	if in.Progress.HasLocation() {
		payload["locationProvided"] = true
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
