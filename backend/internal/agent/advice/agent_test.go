package advice

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/model"
)

type fakeClient struct {
	content string
	err     error
	calls   int
	lastReq model.Request
}

func (f *fakeClient) Chat(_ context.Context, req model.Request) (model.Response, error) {
	f.calls++
	f.lastReq = req
	if f.err != nil {
		return model.Response{}, f.err
	}
	return model.Response{Content: f.content}, nil
}

func ptr[T any](v T) *T { return &v }

// readyState 是一份"数据齐全"的状态：航班状态、时间轴、路程时间都有。
func readyState() domain.State {
	s := domain.NewState()
	s.FlightStatus = domain.FlightStatusOnTime
	s.Gate = ptr("B27")
	s.Timeline = []domain.TimelineNode{
		{Label: "登机口关闭", Time: "2026-09-13T14:45:00+08:00"},
		{Label: "起飞", Time: "2026-09-13T15:00:00+08:00"},
	}
	s.ETAMin = ptr(52)
	s.Traffic = ptr(domain.TrafficHeavy)
	s.Quality = domain.QualityComplete
	return s
}

func TestEvaluate_UsesModelOutputAndInjectsNav(t *testing.T) {
	client := &fakeClient{content: `{
		"stage": "en_route",
		"risk": "yellow",
		"alert": "路况拥堵，建议现在出发",
		"cards": [{"label": "剩余缓冲", "value": "18 分钟"}],
		"actions": [{"id": "leave_now", "title": "尽快出发", "detail": "预计 52 分钟到机场", "nav": "airport"}],
		"reasons": ["航班数据更新于 12:35"]
	}`}

	got := NewAgent(client).Evaluate(context.Background(), Input{
		State:       readyState(),
		AirportIATA: "PVG",
	})

	if got.Stage != domain.StageEnRoute || got.Risk != domain.RiskYellow {
		t.Fatalf("stage/risk = %s/%s", got.Stage, got.Risk)
	}
	if got.Alert == nil || !strings.Contains(*got.Alert, "拥堵") {
		t.Fatalf("alert = %v", got.Alert)
	}
	if len(got.Actions) != 1 {
		t.Fatalf("actions = %d", len(got.Actions))
	}
	// nav 是代码注入的，不是模型给的
	nav := got.Actions[0].Nav
	if nav == nil || !strings.HasPrefix(nav.App, "amapuri://") || !strings.Contains(nav.Web, "uri.amap.com") {
		t.Fatalf("nav 未被正确注入: %+v", nav)
	}
	if !strings.Contains(nav.App, "%E4%B8%8A%E6%B5%B7") { // 上海
		t.Fatalf("目的地应经过 URL 编码: %s", nav.App)
	}
	if len(got.Cards) != 1 || got.Cards[0].Label != "剩余缓冲" {
		t.Fatalf("cards = %+v", got.Cards)
	}
	if client.calls != 1 {
		t.Fatalf("应当只调用一次模型，实际 %d", client.calls)
	}
}

func TestEvaluate_MissingETACannotClaimGreen(t *testing.T) {
	// 模型说"很安全"，但此时没有路程时间，判断不了来不来得及
	client := &fakeClient{content: `{
		"stage": "en_route",
		"risk": "green",
		"alert": null,
		"cards": [],
		"actions": [{"id": "leave_now", "title": "出发", "detail": "现在走", "nav": "airport"}],
		"reasons": []
	}`}

	state := readyState()
	state.ETAMin = nil

	got := NewAgent(client).Evaluate(context.Background(), Input{State: state, AirportIATA: "PVG"})

	if got.Risk != domain.RiskUnknown {
		t.Fatalf("关键数据缺失时必须降级为 unknown，实际 %s", got.Risk)
	}
	if len(got.Actions) != 0 {
		t.Fatalf("降级后不应保留高强度行动，实际 %d 条", len(got.Actions))
	}
	if !hasReason(got.Reasons, "路程时间") {
		t.Fatalf("reasons 应说明降级原因: %v", got.Reasons)
	}
}

func TestEvaluate_RejectsUnknownEnumValues(t *testing.T) {
	client := &fakeClient{content: `{
		"stage": "flying",
		"risk": "green",
		"alert": null,
		"cards": [],
		"actions": [],
		"reasons": []
	}`}

	got := NewAgent(client).Evaluate(context.Background(), Input{State: readyState()})

	if got.Risk != domain.RiskUnknown {
		t.Fatalf("非法枚举应整体降级，实际 risk=%s stage=%s", got.Risk, got.Stage)
	}
	if !hasReason(got.Reasons, "无法识别") {
		t.Fatalf("reasons 应说明原因: %v", got.Reasons)
	}
}

func TestEvaluate_ModelFailureFallsBack(t *testing.T) {
	client := &fakeClient{err: errors.New("connection refused")}

	got := NewAgent(client).Evaluate(context.Background(), Input{State: readyState()})

	if got.Risk != domain.RiskUnknown || len(got.Actions) != 0 {
		t.Fatalf("模型失败时应返回安全兜底，实际 %+v", got)
	}
	if !hasReason(got.Reasons, "调用失败") {
		t.Fatalf("reasons 应说明失败原因: %v", got.Reasons)
	}
}

func TestEvaluate_NilClientStillProducesValidAdvice(t *testing.T) {
	got := NewAgent(nil).Evaluate(context.Background(), Input{State: readyState()})

	if got.Risk != domain.RiskUnknown || got.Stage != domain.StageUnknown {
		t.Fatalf("没有模型时应返回 unknown，实际 risk=%s", got.Risk)
	}
	// 契约要求数组不能是 null
	if got.Cards == nil || got.Actions == nil || got.Reasons == nil {
		t.Fatal("数组字段不能为 nil，序列化后会是 null")
	}
}

func TestEvaluate_ManualStageOverridesModel(t *testing.T) {
	client := &fakeClient{content: `{
		"stage": "en_route",
		"risk": "yellow",
		"alert": null,
		"cards": [],
		"actions": [],
		"reasons": []
	}`}

	state := readyState()
	state.Timeline = []domain.TimelineNode{{Label: "起飞", Time: "2026-09-13T15:00:00+08:00"}}

	got := NewAgent(client).Evaluate(context.Background(), Input{
		State:    state,
		Progress: domain.JourneyProgress{ManualStage: domain.StageSecurity},
	})

	if got.Stage != domain.StageSecurity {
		t.Fatalf("手动确认的阶段应优先，实际 %s", got.Stage)
	}
	if !hasReason(got.Reasons, "手动确认") {
		t.Fatalf("reasons 应说明阶段来源: %v", got.Reasons)
	}
}

func TestEvaluate_UnknownAirportDropsNav(t *testing.T) {
	client := &fakeClient{content: `{
		"stage": "en_route",
		"risk": "yellow",
		"alert": null,
		"cards": [],
		"actions": [{"id": "leave_now", "title": "出发", "detail": "现在走", "nav": "airport"}],
		"reasons": []
	}`}

	got := NewAgent(client).Evaluate(context.Background(), Input{
		State:       readyState(),
		AirportIATA: "ZZZ", // 对照表里没有
	})

	if len(got.Actions) != 1 {
		t.Fatalf("行动应保留，实际 %d 条", len(got.Actions))
	}
	if got.Actions[0].Nav != nil {
		t.Fatalf("查不到机场时不能给导航链接，实际 %+v", got.Actions[0].Nav)
	}
}

func TestBuildAirportNav(t *testing.T) {
	if BuildAirportNav("ZZZ") != nil {
		t.Fatal("未知三字码应返回 nil")
	}
	nav := BuildAirportNav("pvg")
	if nav == nil || !strings.Contains(nav.App, "dname=") {
		t.Fatalf("大小写不敏感地查表失败: %+v", nav)
	}
}

func hasReason(reasons []string, keyword string) bool {
	for _, r := range reasons {
		if strings.Contains(r, keyword) {
			return true
		}
	}
	return false
}

// TestUserPromptCarriesLocationFlag 锁住一件事：阶段判断依赖"有没有定位"，
// 但坐标本身不下发给模型（隐私最小化）。
func TestUserPromptCarriesLocationFlag(t *testing.T) {
	state := readyState()

	t.Run("有定位时带 locationProvided", func(t *testing.T) {
		prompt, err := userPrompt(Input{
			State:    state,
			Progress: domain.JourneyProgress{Location: &domain.Coordinate{Lat: 22.5431, Lng: 114.0579}},
		})
		if err != nil {
			t.Fatalf("构造 prompt 失败: %v", err)
		}
		if !strings.Contains(prompt, `"locationProvided": true`) {
			t.Fatalf("有定位时应下发 locationProvided：\n%s", prompt)
		}
		if strings.Contains(prompt, "114.0579") || strings.Contains(prompt, "22.5431") {
			t.Fatalf("不应把精确坐标下发给模型：\n%s", prompt)
		}
	})

	t.Run("无定位时不带该字段", func(t *testing.T) {
		prompt, err := userPrompt(Input{State: state})
		if err != nil {
			t.Fatalf("构造 prompt 失败: %v", err)
		}
		if strings.Contains(prompt, "locationProvided") {
			t.Fatalf("没有定位时不应出现 locationProvided：\n%s", prompt)
		}
	})
}

// TestUserPromptCarriesBaggage 锁住"托运行李会影响建议"这条输入契约：
// 有无托运必须明确下发给模型，否则建议里永远不会有托运环节。
func TestUserPromptCarriesBaggage(t *testing.T) {
	withBaggage, err := userPrompt(Input{State: readyState(), HasBaggage: true})
	if err != nil {
		t.Fatalf("构造 prompt 失败: %v", err)
	}
	if !strings.Contains(withBaggage, `"hasBaggage": true`) {
		t.Fatalf("带托运行李时应下发 hasBaggage=true：\n%s", withBaggage)
	}

	withoutBaggage, err := userPrompt(Input{State: readyState()})
	if err != nil {
		t.Fatalf("构造 prompt 失败: %v", err)
	}
	if !strings.Contains(withoutBaggage, `"hasBaggage": false`) {
		t.Fatalf("不带行李时也应明确下发 hasBaggage=false：\n%s", withoutBaggage)
	}
}
