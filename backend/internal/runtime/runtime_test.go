package runtime

import (
	"context"
	"errors"
	"strings"
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
