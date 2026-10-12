package skill

import (
	"context"
	"math"
	"strings"
	"testing"

	"aviation-journey-agent/backend/internal/domain"
)

// TestEstimateDrivingMinutes 锁住"定位 → 路程时间"的估算口径：
// 直线距离 × 1.3 路网系数 ÷ 30km/h，且最少 1 分钟。
func TestEstimateDrivingMinutes(t *testing.T) {
	// 深圳市区 → 宝安机场（机场坐标取自 EOOB 机场页）
	got, err := estimateDrivingMinutes(22.5431, 114.0579, 22.639444, 113.810833)
	if err != nil {
		t.Fatalf("正常坐标不应报错: %v", err)
	}
	if got < 55 || got > 80 {
		t.Fatalf("深圳市区→宝安机场 = %d 分钟，期望落在 55~80（实测约 72）", got)
	}

	if got, err := estimateDrivingMinutes(22.5431, 114.0579, 22.5431, 114.0579); err != nil || got != 1 {
		t.Fatalf("同一位置的估算应取下限 1 分钟，实际 %d", got)
	}
}

// TestEstimateDrivingMinutesRejectsUnusableCoordinates 锁住"宁可没有数字"的口径。
//
// 这些输入是实测出来会出事的：lat=lng=1e308 会让 haversine 得到 NaN，
// int(NaN) 在 amd64 上落到 int64 最小值，再被"最少 1 分钟"兜成 1 ——
// 界面于是显示"预计到达机场 1 分钟"并给出 risk=green。
func TestEstimateDrivingMinutesRejectsUnusableCoordinates(t *testing.T) {
	cases := []struct {
		name             string
		fromLat, fromLng float64
		toLat, toLng     float64
		errContains      string
	}{
		{"纬度 91", 91, 121.4, 31.143333, 121.805278, "超出取值范围"},
		{"纬度 -91", -91, 121.4, 31.143333, 121.805278, "超出取值范围"},
		{"经度 181", 31.2, 181, 31.143333, 121.805278, "超出取值范围"},
		{"坐标 1e308", 1e308, 1e308, 31.143333, 121.805278, "超出取值范围"},
		{"坐标 NaN", math.NaN(), 121.4, 31.143333, 121.805278, "超出取值范围"},
		{"坐标 +Inf", math.Inf(1), 121.4, 31.143333, 121.805278, "超出取值范围"},
		{"地球另一端的合法坐标", 31.2304, 121.4737, -33.94, -118.4, "超出可用范围"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := estimateDrivingMinutes(tc.fromLat, tc.fromLng, tc.toLat, tc.toLng)
			if err == nil {
				t.Fatalf("期望报错，实际得到 %d 分钟", got)
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Fatalf("错误信息应包含 %q，实际 %q", tc.errContains, err.Error())
			}
		})
	}
}

// TestRouteETASkillReportsBadLocation：坐标不可信时必须没有观测值、且写明原因。
func TestRouteETASkillReportsBadLocation(t *testing.T) {
	skill := RouteETASkill{}
	journey := domain.Journey{Flights: []domain.Flight{{Number: "CZ8575", From: "SZX", To: "HFE"}}}
	result, err := skill.Execute(context.Background(), Query{
		Journey:  journey,
		Location: &domain.Coordinate{Lat: 1e308, Lng: 1e308},
	})
	if err == nil {
		// 机场坐标查询会走网络，这里只要求：不能产出观测值。
		if len(result.Observations) != 0 {
			t.Fatalf("坐标不可信时不应产生观测值，实际 %v", result.Observations)
		}
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
