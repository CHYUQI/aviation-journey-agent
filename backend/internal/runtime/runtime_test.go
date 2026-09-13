package runtime

import (
	"testing"

	"aviation-journey-agent/backend/internal/domain"
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
