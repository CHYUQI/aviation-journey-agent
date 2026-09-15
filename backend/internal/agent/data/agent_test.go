package data

import (
	"context"
	"testing"

	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/skill"
)

// fakeFlightSkill 同时给出航班状态和到机场耗时，用来验证"已起飞就不再算路程"。
type fakeFlightSkill struct{ status string }

func (f fakeFlightSkill) Name() string { return "fake_flight" }

func (f fakeFlightSkill) Supports(field string) bool {
	return field == skill.FieldFlightStatus || field == skill.FieldTravelETA
}

func (f fakeFlightSkill) Execute(_ context.Context, query skill.Query) (skill.Result, error) {
	result := skill.Result{}
	if f.status != "" {
		result.Observations = append(result.Observations, skill.Observation{
			Field: skill.FieldFlightStatus, Value: f.status,
			Source: "fake", ObservedAt: "2026-09-15T08:00:00+08:00", Confidence: "high",
		})
	}
	if query.Location != nil {
		result.Observations = append(result.Observations, skill.Observation{
			Field: skill.FieldTravelETA, Value: 72,
			Source: "fake", ObservedAt: "2026-09-15T08:00:00+08:00", Confidence: "low",
		})
	}
	return result, nil
}

func TestBuildStateDropsETAWhenFlightFinished(t *testing.T) {
	journey := domain.Journey{
		ID:      "j_1",
		Flights: []domain.Flight{{Number: "HU7851", Date: "2026-09-15", From: "SZX", To: "URC"}},
	}
	progress := domain.JourneyProgress{Location: &domain.Coordinate{Lat: 22.5431, Lng: 114.0579}}

	cases := []struct {
		status     string
		wantETANil bool
	}{
		{domain.FlightStatusDeparted, true},
		{domain.FlightStatusCancelled, true},
		{domain.FlightStatusOnTime, false},
		{domain.FlightStatusScheduled, false},
	}

	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			registry := skill.NewRegistry()
			registry.Register(fakeFlightSkill{status: tc.status})
			agent := NewAgent(registry)

			result, err := agent.BuildState(context.Background(), Input{Journey: journey, Progress: progress})
			if err != nil {
				t.Fatalf("BuildState 失败: %v", err)
			}

			if tc.wantETANil {
				if result.State.ETAMin != nil {
					t.Fatalf("航班 %s 时不应保留到机场耗时，实际 %d", tc.status, *result.State.ETAMin)
				}
				found := false
				for _, issue := range result.Issues {
					if issue == "航班已起飞或取消，到机场的路程时间不再有意义，已忽略" {
						found = true
					}
				}
				if !found {
					t.Fatalf("清掉耗时后应说明原因，实际 issues=%v", result.Issues)
				}
				return
			}

			if result.State.ETAMin == nil || *result.State.ETAMin != 72 {
				t.Fatalf("航班 %s 时应保留到机场耗时 72，实际 %v", tc.status, result.State.ETAMin)
			}
		})
	}
}

// TestBuildTimelineNodes 锁住时间轴节点：登机/关闸/起飞/到达四个节点都要在，
// 且按时间升序；只有起飞时间时不得编造其他节点。
func TestBuildTimelineNodes(t *testing.T) {
	full := buildTimeline(skill.FlightTimes{
		BoardingTime:  "2026-09-15T14:20:00+08:00",
		GateCloseTime: "2026-09-15T14:45:00+08:00",
		DepartureTime: "2026-09-15T15:00:00+08:00",
		ArrivalTime:   "2026-09-15T17:30:00+08:00",
	})
	want := []string{"开始登机", "登机口关闭", "起飞", "到达"}
	if len(full) != len(want) {
		t.Fatalf("时间轴应有 %d 个节点，实际 %d：%+v", len(want), len(full), full)
	}
	for i, label := range want {
		if full[i].Label != label {
			t.Fatalf("第 %d 个节点应为 %q，实际 %q", i+1, label, full[i].Label)
		}
	}

	onlyDeparture := buildTimeline(skill.FlightTimes{DepartureTime: "2026-09-15T15:00:00+08:00"})
	if len(onlyDeparture) != 1 || onlyDeparture[0].Label != "起飞" {
		t.Fatalf("只有起飞时间时应只有一个节点，实际 %+v", onlyDeparture)
	}
}
