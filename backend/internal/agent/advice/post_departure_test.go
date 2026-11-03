package advice

import (
	"context"
	"testing"

	"aviation-journey-agent/backend/internal/domain"
)

func TestEvaluate_PostDepartureSkipsPreflightAdvice(t *testing.T) {
	for _, status := range []string{domain.FlightStatusDeparted, domain.FlightStatusDiverted} {
		for _, configured := range []bool{false, true} {
			t.Run(status+map[bool]string{false: "/no_model", true: "/model"}[configured], func(t *testing.T) {
				client := &fakeClient{content: `{"stage":"en_route","risk":"red"}`}
				agent := NewAgent(nil)
				if configured {
					agent = NewAgent(client)
				}
				state := domain.NewState()
				state.FlightStatus = status
				got := agent.Evaluate(context.Background(), Input{
					State:    state,
					Progress: domain.JourneyProgress{ManualStage: domain.StageEnRoute},
					Issues:   []string{"机场数据暂不可用"},
				})
				wantStage := domain.StageDeparted
				if status == domain.FlightStatusDiverted {
					wantStage = domain.StageDisrupted
				}
				if got.Risk != domain.RiskUnknown || got.Stage != wantStage {
					t.Fatalf("stage/risk = %s/%s", got.Stage, got.Risk)
				}
				if got.Alert != nil || got.Cards == nil || len(got.Cards) != 0 || got.Actions == nil || len(got.Actions) != 0 {
					t.Fatalf("unexpected preflight advice: %+v", got)
				}
				if len(got.Reasons) != 2 || got.Reasons[0] != "机场数据暂不可用" {
					t.Fatalf("missing reasons: %v", got.Reasons)
				}
				if client.calls != 0 {
					t.Fatal("post-departure advice must not call the model")
				}
				cached := domain.NewAdvice()
				cached.Risk = domain.RiskRed
				cached.Alert = ptr("立即出发")
				cached.Cards = []domain.Card{{Label: "剩余缓冲", Value: "0 分钟"}}
				cached.Actions = []domain.Action{{ID: "leave_now", Title: "立即出发"}}
				refreshed := WithAirportTimeSimulation(cached, Input{State: state})
				if refreshed.Risk != domain.RiskUnknown || refreshed.Stage != wantStage || refreshed.Alert != nil || len(refreshed.Cards) != 0 || len(refreshed.Actions) != 0 {
					t.Fatalf("refresh retained preflight advice: %+v", refreshed)
				}
			})
		}
	}
}
