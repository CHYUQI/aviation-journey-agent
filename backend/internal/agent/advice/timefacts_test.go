package advice

import (
	"testing"
	"time"

	"aviation-journey-agent/backend/internal/domain"
)

func cst() *time.Location { return time.FixedZone("CST", 8*3600) }

func farFutureState() domain.State {
	return domain.State{
		FlightStatus: domain.FlightStatusScheduled,
		Timeline: []domain.TimelineNode{
			{Label: "开始登机", Time: "2026-09-22T18:10:00+08:00"},
			{Label: "登机口关闭", Time: "2026-09-22T18:35:00+08:00"},
			{Label: "起飞", Time: "2026-09-22T18:50:00+08:00"},
			{Label: "到达", Time: "2026-09-22T20:55:00+08:00"},
		},
	}
}

// TestComputeTimeFactsFarFutureFlight 是用户报的那个 case：
// 10:09 上报定位、到机场 7 分钟、18:35 关闸 —— 缓冲 8 小时 19 分，绝不该判成"非常紧张"。
func TestComputeTimeFactsFarFutureFlight(t *testing.T) {
	now := time.Date(2026, 9, 22, 10, 9, 0, 0, cst())
	eta := 7

	facts := ComputeTimeFacts(farFutureState(), domain.JourneyProgress{
		Location: &domain.Coordinate{Lat: 39.9042, Lng: 116.4074},
	}, &eta, now)

	if facts.ServerNow != "2026-09-22T10:09:00+08:00" {
		t.Fatalf("ServerNow = %q", facts.ServerNow)
	}
	if facts.MinutesUntilGateClose == nil || *facts.MinutesUntilGateClose != 506 {
		t.Fatalf("距登机口关闭应为 506 分钟（8h26m），实际 %v", facts.MinutesUntilGateClose)
	}
	if facts.BufferMin == nil || *facts.BufferMin != 499 {
		t.Fatalf("缓冲应为 499 分钟（506 - 7），实际 %v", facts.BufferMin)
	}
}

// TestComputeTimeFactsAtAirport：旅客已手动确认在机场，不需要再扣路程时间。
func TestComputeTimeFactsAtAirport(t *testing.T) {
	now := time.Date(2026, 9, 22, 18, 0, 0, 0, cst())

	facts := ComputeTimeFacts(farFutureState(), domain.JourneyProgress{ManualStage: domain.StageSecurity}, nil, now)

	if facts.BufferMin == nil || *facts.BufferMin != 35 {
		t.Fatalf("在机场安检时缓冲应为 35 分钟（18:35 - 18:00），实际 %v", facts.BufferMin)
	}
}

// TestComputeTimeFactsWithoutLocation：没有定位也没有手动阶段时，
// 缓冲算不出来（必须留空），但距关闸时间仍然可算。
func TestComputeTimeFactsWithoutLocation(t *testing.T) {
	now := time.Date(2026, 9, 22, 10, 9, 0, 0, cst())

	facts := ComputeTimeFacts(farFutureState(), domain.JourneyProgress{}, nil, now)

	if facts.BufferMin != nil {
		t.Fatalf("没有定位时缓冲必须留空，实际 %d", *facts.BufferMin)
	}
	if facts.MinutesUntilGateClose == nil || *facts.MinutesUntilGateClose != 506 {
		t.Fatalf("距登机口关闭仍应算出 506 分钟，实际 %v", facts.MinutesUntilGateClose)
	}
}

// TestComputeTimeFactsPastGateClose：已经过了关闸，缓冲必须是负数（用来判 red）。
func TestComputeTimeFactsPastGateClose(t *testing.T) {
	now := time.Date(2026, 9, 22, 19, 0, 0, 0, cst())
	eta := 10

	facts := ComputeTimeFacts(farFutureState(), domain.JourneyProgress{
		Location: &domain.Coordinate{Lat: 39.9042, Lng: 116.4074},
	}, &eta, now)

	if facts.BufferMin == nil || *facts.BufferMin != -35 {
		t.Fatalf("关闸后缓冲应为 -35 分钟（18:35 - 19:00 - 10），实际 %v", facts.BufferMin)
	}
}

// TestGateCloseDeadlineFallsBackToDeparture：没有"登机口关闭"节点时退回"起飞"节点。
func TestGateCloseDeadlineFallsBackToDeparture(t *testing.T) {
	state := domain.State{Timeline: []domain.TimelineNode{
		{Label: "起飞", Time: "2026-09-22T18:50:00+08:00"},
	}}
	now := time.Date(2026, 9, 22, 18, 0, 0, 0, cst())

	facts := ComputeTimeFacts(state, domain.JourneyProgress{ManualStage: domain.StageWaiting}, nil, now)

	if facts.MinutesUntilGateClose == nil || *facts.MinutesUntilGateClose != 50 {
		t.Fatalf("应退回起飞节点算 50 分钟，实际 %v", facts.MinutesUntilGateClose)
	}
}
