package advice

import (
	"context"

	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/model"
)

// Input 是 AdviceAgent 的输入。
type Input struct {
	State    domain.State
	Progress domain.JourneyProgress
	// Issues 是取数阶段发现的问题，会一并写进 reasons
	Issues []string
	// AirportIATA 用于生成"导航到机场"的跳转链接
	AirportIATA string
}

// Agent 根据行程状态生成行动建议。
//
// 数据流：State -> 大模型 -> 结构校验 -> 枚举校验 -> 风险兜底 -> Advice。
// 任何一步失败都会降级成"risk = unknown + 说明原因"，而不是让整个请求失败。
type Agent struct {
	client model.Client
}

// NewAgent 构造建议 Agent。
//
// client 允许为 nil：此时只走降级路径，服务照常启动。
// 这样在没有 API key 的环境里也能跑通整条链路。
func NewAgent(client model.Client) *Agent { return &Agent{client: client} }

// Evaluate 生成行动建议。
func (a *Agent) Evaluate(ctx context.Context, in Input) domain.Advice {
	result := a.evaluate(ctx, in)

	// 旅客手动确认的阶段是事实，优先于模型判断
	if in.Progress.ManualStage != "" {
		result.Stage = in.Progress.ManualStage
		result.Reasons = append(result.Reasons, "当前阶段由旅客手动确认，上报定位后自动解除")
	}

	// 最后一道防线：数据不足时不允许给出"安全"结论
	enforceRiskFloor(&result, in.State)

	return result
}

func (a *Agent) evaluate(ctx context.Context, in Input) domain.Advice {
	if a.client == nil {
		return a.degraded(in, "模型未配置，暂时无法生成行动建议")
	}

	prompt, err := userPrompt(in)
	if err != nil {
		return a.degraded(in, "构造模型输入失败："+err.Error())
	}

	var parsed modelAdvice
	_, err = model.GenerateJSON(ctx, a.client, model.Request{
		Messages: []model.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
	}, &parsed)
	if err != nil {
		return a.degraded(in, "行动决策模型调用失败："+err.Error())
	}

	// 枚举白名单校验：模型可能返回自造的阶段名或风险等级
	if !parsed.valid() {
		return a.degraded(in, "模型返回了无法识别的阶段或风险等级，已降级处理")
	}

	// 不在这里追加 in.Issues：prompt 已要求模型把 dataIssues 用人话写进 reasons，
	// 再追加一遍会在界面上出现内容相近的两条。模型没接上时才走 degraded 分支。
	return parsed.toDomain(BuildAirportNav(in.AirportIATA))
}

// degraded 返回一份安全的兜底建议：risk = unknown、没有行动，
// 但把原因写清楚，让旅客知道为什么没有建议。
func (a *Agent) degraded(in Input, reason string) domain.Advice {
	result := domain.NewAdvice()
	result.Reasons = append(result.Reasons, in.Issues...)
	result.Reasons = append(result.Reasons, reason)
	return result
}
