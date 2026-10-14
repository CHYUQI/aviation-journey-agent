package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	adviceagent "aviation-journey-agent/backend/internal/agent/advice"
	dataagent "aviation-journey-agent/backend/internal/agent/data"
	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/model"
	"aviation-journey-agent/backend/internal/store"
)

func ptr[T any](v T) *T { return &v }

func TestSameState(t *testing.T) {
	base := domain.NewState()
	base.FlightStatus = domain.FlightStatusOnTime
	base.ETAMin = ptr(52)
	base.Timeline = []domain.TimelineNode{{Label: "起飞", Time: "2026-09-13T15:00:00+08:00"}}
	base.UpdatedAt = "2026-09-13T20:00:00+08:00"

	t.Run("只有时间戳不同视为相同", func(t *testing.T) {
		other := base
		other.UpdatedAt = "2026-09-13T20:00:15+08:00"
		if !sameState(base, other) {
			t.Fatal("只有时间戳不同时应判定为无变化，可以复用上一版建议")
		}
	})

	t.Run("内容变化视为不同", func(t *testing.T) {
		other := base
		other.ETAMin = ptr(30)
		if sameState(base, other) {
			t.Fatal("路程时间变化时必须重新生成建议")
		}
	})

	t.Run("从有到无视为不同", func(t *testing.T) {
		other := base
		other.ETAMin = nil
		if sameState(base, other) {
			t.Fatal("数据丢失时必须重新生成建议")
		}
	})

	t.Run("时间轴变化视为不同", func(t *testing.T) {
		other := base
		other.Timeline = append([]domain.TimelineNode{}, base.Timeline...)
		other.Timeline[0].Time = "2026-09-13T16:00:00+08:00"
		if sameState(base, other) {
			t.Fatal("航班时间变化时必须重新生成建议")
		}
	})
}

func TestDepartureAirport(t *testing.T) {
	journey := domain.Journey{
		Flights: []domain.Flight{{Number: "CA1234", From: "PVG", To: "PEK"}},
	}
	if got := departureAirport(journey); got != "PVG" {
		t.Fatalf("departureAirport = %q", got)
	}
	if got := departureAirport(domain.Journey{}); got != "" {
		t.Fatalf("没有航段时应返回空字符串，实际 %q", got)
	}
}

// failingModelClient 模拟"模型调用失败"，用来复现刷新时降级成空建议的情况。
type failingModelClient struct{}

func (failingModelClient) Chat(context.Context, model.Request) (model.Response, error) {
	return model.Response{}, errors.New("context deadline exceeded")
}

func TestFlightDatePassed(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.Local)

	cases := []struct {
		name string
		date string
		want bool
	}{
		{"出发当天不算过去", "2026-09-15", false},
		{"明天不算过去", "2026-09-16", false},
		{"昨天算过去", "2026-09-14", true},
		{"没有日期不算过去", "", false},
		{"日期非法不算过去", "不是日期", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			journey := domain.Journey{Flights: []domain.Flight{{Number: "CZ3101", Date: tc.date}}}
			if got := flightDatePassed(journey, now); got != tc.want {
				t.Fatalf("flightDatePassed(%q) = %v, want %v", tc.date, got, tc.want)
			}
		})
	}
}

func TestShouldRefreshSkipsFinishedJourneys(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.Local)

	today := domain.Journey{ID: "j_today", Flights: []domain.Flight{{Number: "CZ3101", Date: "2026-09-15"}}}
	past := domain.Journey{ID: "j_past", Flights: []domain.Flight{{Number: "CZ3101", Date: "2026-09-13"}}}

	memoryStore := store.NewMemoryStore()
	memoryStore.SaveJourney(today)
	memoryStore.SaveSnapshot(domain.JourneySnapshot{Journey: today, State: domain.State{FlightStatus: domain.FlightStatusScheduled}})
	memoryStore.SaveJourney(past)
	memoryStore.SaveSnapshot(domain.JourneySnapshot{Journey: past, State: domain.State{FlightStatus: domain.FlightStatusScheduled}})

	rt := New(memoryStore, nil, nil, NewSSEHub())

	if !rt.shouldRefresh("j_today", now) {
		t.Error("出发当天的行程仍应刷新")
	}
	if rt.shouldRefresh("j_past", now) {
		t.Error("出发日期已过的行程不该再刷新")
	}

	memoryStore.SaveSnapshot(domain.JourneySnapshot{Journey: today, State: domain.State{FlightStatus: domain.FlightStatusDeparted}})
	if rt.shouldRefresh("j_today", now) {
		t.Error("已起飞的行程不该再刷新")
	}
}

func TestDecideKeepsAdviceWhenRefreshDegrades(t *testing.T) {
	journey := domain.Journey{
		ID:      "j_1",
		Flights: []domain.Flight{{Number: "CZ3101", Date: "2026-09-15", From: "CAN", To: "PKX"}},
	}
	previous := domain.JourneySnapshot{
		Status:  domain.StatusReady,
		Journey: journey,
		State:   domain.State{FlightStatus: domain.FlightStatusScheduled},
		Advice: domain.Advice{
			Stage:   domain.StageEnRoute,
			Risk:    domain.RiskYellow,
			Alert:   ptr("安检排队上升到 45 分钟，缓冲可能不足"),
			Cards:   []domain.Card{{Label: "剩余缓冲", Value: "18 分钟"}},
			Actions: []domain.Action{{ID: "leave_now", Title: "尽快出发"}},
			Reasons: []string{"航班数据更新于 12:35，来源 flight_status"},
		},
	}
	rt := New(store.NewMemoryStore(), nil, adviceagent.NewAgent(failingModelClient{}), NewSSEHub())

	// 这一轮数据源失败，State 与上一轮不同，会触发重新决策且模型调用失败
	result := dataagent.Result{
		State:  domain.State{FlightStatus: domain.FlightStatusUnknown},
		Issues: []string{"EOOB 查询失败：HTTP 403"},
	}
	got := rt.decide(context.Background(), result, domain.JourneyProgress{}, journey, previous, true)

	if len(got.Cards) != 1 || len(got.Actions) != 1 {
		t.Fatalf("刷新降级时不该清空上一轮建议：cards=%d actions=%d", len(got.Cards), len(got.Actions))
	}
	if *got.Alert != *previous.Advice.Alert {
		t.Errorf("提醒应保留，实际 %q", *got.Alert)
	}
	if got.Risk != domain.RiskUnknown {
		t.Errorf("数据缺失时风险必须降为 unknown，实际 %q", got.Risk)
	}
	joined := strings.Join(got.Reasons, " | ")
	if !strings.Contains(joined, "航班数据更新于 12:35") || !strings.Contains(joined, "EOOB 查询失败") {
		t.Errorf("reasons 应同时保留旧依据与新问题，实际 %q", joined)
	}
}

func TestDecideRefreshesAirportSimulationWhenModelFails(t *testing.T) {
	journey := domain.Journey{ID: "j_simulation", HasBaggage: true}
	state := domain.NewState()
	state.FlightStatus = domain.FlightStatusOnTime
	state.Timeline = []domain.TimelineNode{{Label: "起飞", Time: "2026-10-13T18:00:00+08:00"}}
	state.Guide = []string{"客流（出发）：当前小时计划旅客座位数 1000 座，未来 24 小时合计约 24000 座"}
	base := domain.NewAdvice()
	base.Stage = domain.StageAtAirport
	base.Risk = domain.RiskYellow
	base.Actions = []domain.Action{{ID: "security", Title: "办理机场手续", Detail: "按现场指引前往安检，再去登机口"}}
	previous := domain.JourneySnapshot{
		Status: domain.StatusReady,
		State:  state,
		Advice: adviceagent.WithAirportTimeSimulation(base, adviceagent.Input{State: state, HasBaggage: true}),
	}

	for _, tc := range []struct {
		name, traffic  string
		wantSimulation bool
	}{
		{"客流升高且阶段推进", "客流（出发）：当前小时计划旅客座位数 2000 座，未来 24 小时合计约 24000 座", true},
		{"当前小时缺失", "客流（出发）：当前小时数据暂缺，未来 24 小时合计约 24000 座", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := New(store.NewMemoryStore(), nil, adviceagent.NewAgent(failingModelClient{}), NewSSEHub())
			nextState := state
			nextState.Guide = []string{tc.traffic}
			got := rt.decide(context.Background(), dataagent.Result{State: nextState},
				domain.JourneyProgress{ManualStage: domain.StageSecurity}, journey, previous, true)
			if got.Stage != domain.StageSecurity || len(got.Actions) != 1 || len(got.Cards) != 0 {
				t.Fatalf("应保留已有行动并应用最新手动阶段：%+v", got)
			}
			detail := got.Actions[0].Detail
			if tc.wantSimulation {
				if !strings.Contains(detail, "安检时间（模拟）：约 40 分钟") || strings.Contains(detail, "值机时间（模拟）") {
					t.Fatalf("降级复用仍应按新客流和阶段更新模拟：%s", detail)
				}
			} else if detail != base.Actions[0].Detail {
				t.Fatalf("当前小时客流缺失时应清除旧模拟：%s", detail)
			}
			if strings.Count(strings.Join(got.Reasons, " | "), "机场流程模拟依据：") != 1 {
				t.Fatalf("旧模拟依据应被替换：%v", got.Reasons)
			}
			if !strings.Contains(previous.Advice.Actions[0].Detail, "安检时间（模拟）：约 20 分钟") {
				t.Fatal("刷新不得修改此前快照")
			}
		})
	}
}

// recordingModelClient 记下最后一次提示词，用来断言模型看到的输入内容。
type recordingModelClient struct{ prompt string }

func (c *recordingModelClient) Chat(_ context.Context, req model.Request) (model.Response, error) {
	for _, m := range req.Messages {
		if m.Role == "user" {
			c.prompt = m.Content
		}
	}
	return model.Response{
		Content: `{"stage":"en_route","risk":"unknown","alert":null,"cards":[],"actions":[],"reasons":["测试"]}`,
	}, nil
}

func TestRecalculateSendsStampedStateToAdvice(t *testing.T) {
	memoryStore := store.NewMemoryStore()
	journey := domain.Journey{
		ID:      "j_stamp",
		Flights: []domain.Flight{{Number: "CZ3101", Date: "2026-09-15", From: "CAN", To: "PKX"}},
	}
	memoryStore.SaveJourney(journey)
	memoryStore.SaveProgress("j_stamp", domain.JourneyProgress{})

	recorder := &recordingModelClient{}
	rt := New(memoryStore, dataagent.NewAgent(nil), adviceagent.NewAgent(recorder), NewSSEHub())

	snapshot, err := rt.Recalculate(context.Background(), "j_stamp")
	if err != nil {
		t.Fatalf("Recalculate 失败: %v", err)
	}
	if recorder.prompt == "" {
		t.Fatal("没有捕获到模型提示词")
	}
	if strings.Contains(recorder.prompt, `"updatedAt": ""`) {
		t.Fatalf("模型输入里的 state.updatedAt 不能为空（模型会据此说\"更新时间未知\"）：\n%s", recorder.prompt)
	}
	if snapshot.State.UpdatedAt == "" || !strings.Contains(recorder.prompt, snapshot.State.UpdatedAt) {
		t.Fatalf("快照时间戳与模型看到的不一致：snapshot=%q", snapshot.State.UpdatedAt)
	}
}

// blockingModelClient 让第一轮分析停在模型调用里，用来复现
// "分析进行中旅客又上报了一次阶段"的竞态。
type blockingModelClient struct {
	entered chan struct{}
	release chan struct{}

	mu    sync.Mutex
	calls int
}

func (c *blockingModelClient) Chat(ctx context.Context, _ model.Request) (model.Response, error) {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()

	if call == 1 {
		close(c.entered)
		select {
		case <-c.release:
		case <-ctx.Done():
			return model.Response{}, ctx.Err()
		}
	}
	return model.Response{
		Content: `{"stage":"unknown","risk":"unknown","alert":null,"cards":[],"actions":[],"reasons":["测试"]}`,
	}, nil
}

func (c *blockingModelClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// TestRecalculateRerunsWhenRequestArrivesDuringAnalysis 锁住这次实测出来的问题：
// 分析进行中再上报一次阶段，旧实现直接 return 把这次重算丢掉了，
// 界面会一直停在旧阶段（实测等到 4.5 分钟之后仍是 waiting，最后一次上报是 security）。
func TestRecalculateRerunsWhenRequestArrivesDuringAnalysis(t *testing.T) {
	memoryStore := store.NewMemoryStore()
	journey := domain.Journey{
		ID:      "j_dirty",
		Flights: []domain.Flight{{Number: "CZ3101", Date: "2026-09-15", From: "CAN", To: "PKX"}},
	}
	memoryStore.SaveJourney(journey)
	memoryStore.SaveProgress("j_dirty", domain.JourneyProgress{})
	// CreateJourney 会先落一份 processing 快照，这里保持同样的前提
	memoryStore.SaveSnapshot(domain.JourneySnapshot{
		Status:  domain.StatusProcessing,
		Journey: journey,
		State:   domain.NewState(),
		Advice:  domain.NewAdvice(),
	})

	client := &blockingModelClient{entered: make(chan struct{}), release: make(chan struct{})}
	rt := New(memoryStore, dataagent.NewAgent(nil), adviceagent.NewAgent(client), NewSSEHub())

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := rt.Recalculate(context.Background(), "j_dirty"); err != nil {
			t.Errorf("第一轮 Recalculate 报错: %v", err)
		}
	}()

	select {
	case <-client.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("第一轮分析没有进入模型调用")
	}

	// 分析进行中：旅客手动确认了安检阶段，并触发一次重算
	memoryStore.SaveProgress("j_dirty", domain.JourneyProgress{ManualStage: domain.StageSecurity})
	if _, err := rt.Recalculate(context.Background(), "j_dirty"); err != nil {
		t.Fatalf("分析进行中的重算请求不应报错: %v", err)
	}

	close(client.release)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("第一轮分析结束后没有补跑")
	}

	if got := client.callCount(); got != 2 {
		t.Fatalf("分析进行中的上报必须补跑一次，模型调用次数 = %d，期望 2", got)
	}
	snapshot, err := memoryStore.GetSnapshot("j_dirty")
	if err != nil {
		t.Fatalf("读取快照失败: %v", err)
	}
	if snapshot.Advice.Stage != domain.StageSecurity {
		t.Fatalf("补跑后快照阶段 = %q，期望 %q（最后一次上报必须体现在最终快照里）",
			snapshot.Advice.Stage, domain.StageSecurity)
	}
}

// TestSameAdviceInputsIncludesManualStage 说明为什么不能只比较 State：
// 手动阶段只影响 advice，不影响 state，漏掉它就会复用旧建议。
func TestSameAdviceInputsIncludesManualStage(t *testing.T) {
	state := domain.State{FlightStatus: domain.FlightStatusOnTime, UpdatedAt: "2026-09-15T10:00:00+08:00"}
	base := adviceInputs{state: state, manualStage: domain.StageWaiting}

	sameStamp := base
	sameStamp.state.UpdatedAt = "2026-09-15T10:00:15+08:00"
	if !sameAdviceInputs(base, sameStamp) {
		t.Fatal("只有时间戳不同时应视为同一份输入")
	}

	otherStage := base
	otherStage.manualStage = domain.StageSecurity
	if sameAdviceInputs(base, otherStage) {
		t.Fatal("旅客改了阶段必须重新生成建议")
	}

	otherIssues := base
	otherIssues.issues = []string{"缺少定位，无法估算路程时间"}
	if sameAdviceInputs(base, otherIssues) {
		t.Fatal("取数问题变化时必须重新生成建议")
	}
}
