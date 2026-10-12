package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func validFlight() Flight {
	return Flight{Number: "CA1234", Date: "2026-09-23", From: "PVG", To: "PEK"}
}

// TestCreateJourneyRequestValidate 锁住 POST /journey 的入参口径：
// 契约里写了的 required / pattern / format 必须在边界上就挡住，
// 不能靠下游技能"查不到就算了"。
func TestCreateJourneyRequestValidate(t *testing.T) {
	baggage := true

	cases := []struct {
		name    string
		request CreateJourneyRequest
		wantErr string
	}{
		{"正常请求", CreateJourneyRequest{Flights: []Flight{validFlight()}, HasBaggage: &baggage}, ""},
		{"缺 hasBaggage", CreateJourneyRequest{Flights: []Flight{validFlight()}}, "hasBaggage"},
		{"flights 为空", CreateJourneyRequest{HasBaggage: &baggage}, "至少需要一个航段"},
		{"缺航班号", CreateJourneyRequest{Flights: []Flight{{Date: "2026-09-23", From: "PVG", To: "PEK"}}, HasBaggage: &baggage}, "必填"},
		{"航班号带空格", CreateJourneyRequest{Flights: []Flight{{Number: "CA 1234", Date: "2026-09-23", From: "PVG", To: "PEK"}}, HasBaggage: &baggage}, "航班号"},
		{"航班号小写", CreateJourneyRequest{Flights: []Flight{{Number: "ca1234", Date: "2026-09-23", From: "PVG", To: "PEK"}}, HasBaggage: &baggage}, "航班号"},
		{"航班号只有数字", CreateJourneyRequest{Flights: []Flight{{Number: "1234", Date: "2026-09-23", From: "PVG", To: "PEK"}}, HasBaggage: &baggage}, "航班号"},
		{"日期未补零", CreateJourneyRequest{Flights: []Flight{{Number: "CA1234", Date: "2026-9-1", From: "PVG", To: "PEK"}}, HasBaggage: &baggage}, "日期"},
		{"日期不存在", CreateJourneyRequest{Flights: []Flight{{Number: "CA1234", Date: "2026-13-45", From: "PVG", To: "PEK"}}, HasBaggage: &baggage}, "日期"},
		{"日期带时间", CreateJourneyRequest{Flights: []Flight{{Number: "CA1234", Date: "2026-09-23T00:00:00+08:00", From: "PVG", To: "PEK"}}, HasBaggage: &baggage}, "日期"},
		{"出发机场小写", CreateJourneyRequest{Flights: []Flight{{Number: "CA1234", Date: "2026-09-23", From: "pvg", To: "PEK"}}, HasBaggage: &baggage}, "出发机场"},
		{"到达机场四位", CreateJourneyRequest{Flights: []Flight{{Number: "CA1234", Date: "2026-09-23", From: "PVG", To: "PEKK"}}, HasBaggage: &baggage}, "到达机场"},
		{"出发机场纯空格", CreateJourneyRequest{Flights: []Flight{{Number: "CA1234", Date: "2026-09-23", From: "   ", To: "PEK"}}, HasBaggage: &baggage}, "出发机场"},
		{"出发机场带换行", CreateJourneyRequest{Flights: []Flight{{Number: "CA1234", Date: "2026-09-23", From: "PVG\nPEK", To: "PEK"}}, HasBaggage: &baggage}, "出发机场"},
		{"超长航班号", CreateJourneyRequest{Flights: []Flight{{Number: strings.Repeat("A", 20000), Date: "2026-09-23", From: "PVG", To: "PEK"}}, HasBaggage: &baggage}, "航班号"},
		{"第二个航段非法", CreateJourneyRequest{
			Flights:    []Flight{validFlight(), {Number: "CA1234", Date: "2026-09-23", From: "PVG", To: "pvg"}},
			HasBaggage: &baggage,
		}, "第 2 个航段"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.request.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("期望通过，实际报错：%v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("期望报错（包含 %q），实际通过", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("错误信息应包含 %q，实际 %q", tc.wantErr, err.Error())
			}
		})
	}
}

// TestCreateJourneyRequestHasBaggageRequired 走一遍 JSON 反序列化：
// 缺字段和显式 null 都必须被当成"没给"，不能默认成 false。
func TestCreateJourneyRequestHasBaggageRequired(t *testing.T) {
	body := `{"flights":[{"number":"CA1234","date":"2026-09-23","from":"PVG","to":"PEK"}]}`

	for _, tc := range []struct {
		name string
		raw  string
		ok   bool
	}{
		{"缺 hasBaggage", body, false},
		{"hasBaggage 为 null", strings.Replace(body, "}]}", "}],\"hasBaggage\":null}", 1), false},
		{"hasBaggage 为 false", strings.Replace(body, "}]}", "}],\"hasBaggage\":false}", 1), true},
		{"hasBaggage 为 true", strings.Replace(body, "}]}", "}],\"hasBaggage\":true}", 1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req CreateJourneyRequest
			if err := json.Unmarshal([]byte(tc.raw), &req); err != nil {
				t.Fatalf("反序列化失败: %v", err)
			}
			err := req.Validate()
			if tc.ok && err != nil {
				t.Fatalf("期望通过，实际 %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("期望 400，实际通过")
			}
		})
	}
}

// TestUpdateLocationRequestValidate 覆盖定位上报的两种用法与边界。
func TestUpdateLocationRequestValidate(t *testing.T) {
	lat, lng := 31.2304, 121.4737
	stage := StageCheckIn
	badStage := "departed"

	cases := []struct {
		name    string
		request UpdateLocationRequest
		wantErr string
	}{
		{"只给坐标", UpdateLocationRequest{Lat: &lat, Lng: &lng}, ""},
		{"只给阶段", UpdateLocationRequest{Stage: &stage}, ""},
		{"坐标+阶段", UpdateLocationRequest{Lat: &lat, Lng: &lng, Stage: &stage}, ""},
		{"只给 lat", UpdateLocationRequest{Lat: &lat}, "成对"},
		{"只给 lng", UpdateLocationRequest{Lng: &lng}, "成对"},
		{"都不给", UpdateLocationRequest{}, "至少提供"},
		{"阶段不在白名单", UpdateLocationRequest{Stage: &badStage}, "不合法"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.request.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("期望通过，实际报错：%v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("期望错误包含 %q，实际 %v", tc.wantErr, err)
			}
		})
	}
}
