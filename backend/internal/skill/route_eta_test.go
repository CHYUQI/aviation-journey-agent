package skill

import (
	"context"
	"strings"
	"testing"

	"aviation-journey-agent/backend/internal/domain"
)

// TestEstimateDrivingMinutes 锁住"定位 → 路程时间"的估算口径：
// 直线距离 × 1.3 路网系数 ÷ 30km/h，且最少 1 分钟。
func TestEstimateDrivingMinutes(t *testing.T) {
	// 深圳市区 → 宝安机场（机场坐标取自 EOOB 机场页）
	got := estimateDrivingMinutes(22.5431, 114.0579, 22.639444, 113.810833)
	if got < 55 || got > 80 {
		t.Fatalf("深圳市区→宝安机场 = %d 分钟，期望落在 55~80（实测约 72）", got)
	}

	if got := estimateDrivingMinutes(22.5431, 114.0579, 22.5431, 114.0579); got != 1 {
		t.Fatalf("同一位置的估算应取下限 1 分钟，实际 %d", got)
	}
}

// TestRouteETASkillWithoutLocation：没定位时必须如实报原因，不能瞎算。
func TestRouteETASkillWithoutLocation(t *testing.T) {
	result, err := RouteETASkill{}.Execute(context.Background(), Query{
		Journey: domain.Journey{Flights: []domain.Flight{{Number: "CZ8575", From: "SZX", To: "HFE"}}},
	})
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if len(result.Observations) != 0 {
		t.Fatalf("没有定位时不应产生观测值，实际 %d 条", len(result.Observations))
	}
	if len(result.Issues) == 0 || !strings.Contains(result.Issues[0], "缺少定位") {
		t.Fatalf("没有定位时应说明原因，实际 issues=%v", result.Issues)
	}
}

// TestHaversineApproximatesKnownDistance：深圳→广州直线约 100km，防止公式写错。
func TestHaversineApproximatesKnownDistance(t *testing.T) {
	got := haversineKm(22.5431, 114.0579, 23.1291, 113.2644)
	if got < 90 || got > 112 {
		t.Fatalf("深圳→广州 = %.1f km，期望约 100km", got)
	}
}
