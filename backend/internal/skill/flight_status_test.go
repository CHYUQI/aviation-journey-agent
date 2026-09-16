package skill

import (
	"strings"
	"testing"

	"aviation-journey-agent/backend/internal/domain"
)

// TestDeriveBoardingTimes 锁住登机/关闸的推算口径：
// 计划起飞前 40 分钟开始登机、前 15 分钟关闭登机口（与契约示例一致）。
func TestDeriveBoardingTimes(t *testing.T) {
	boarding, gateClose, ok := deriveBoardingTimes("2026-09-15T15:00:00+08:00")
	if !ok {
		t.Fatal("可解析的起飞时间应返回 ok=true")
	}
	if boarding != "2026-09-15T14:20:00+08:00" {
		t.Fatalf("开始登机 = %s，期望 2026-09-15T14:20:00+08:00", boarding)
	}
	if gateClose != "2026-09-15T14:45:00+08:00" {
		t.Fatalf("登机口关闭 = %s，期望 2026-09-15T14:45:00+08:00", gateClose)
	}

	if _, _, ok := deriveBoardingTimes(""); ok {
		t.Error("空时间不应推算")
	}
	if _, _, ok := deriveBoardingTimes("不是时间"); ok {
		t.Error("非法时间不应推算")
	}
}

// TestPickSegment 锁住经停航班的取段规则：
// 填整段（TSN→SWA）时取"出发地相同"的第一段；填尾段时取"目的地相同"的那段。
func TestPickSegment(t *testing.T) {
	segments := []ResolvedFlightIdentity{
		{Number: "CZ6656", From: "TSN", To: "YIW", Date: "2026-09-15"},
		{Number: "CZ6656", From: "YIW", To: "SWA", Date: "2026-09-15"},
	}

	segment, ok := pickSegment(segments, domain.Flight{Number: "CZ6656", From: "TSN", To: "SWA"})
	if !ok || segment.From != "TSN" || segment.To != "YIW" {
		t.Fatalf("整段输入应取首段 TSN→YIW，实际 %+v ok=%v", segment, ok)
	}

	segment, ok = pickSegment(segments, domain.Flight{Number: "CZ6656", From: "ZZZ", To: "SWA"})
	if !ok || segment.From != "YIW" || segment.To != "SWA" {
		t.Fatalf("只匹配目的地时应取 YIW→SWA，实际 %+v ok=%v", segment, ok)
	}

	single := []ResolvedFlightIdentity{{Number: "MF8822", From: "HRB", To: "NTG", Date: "2026-09-15"}}
	if segment, ok := pickSegment(single, domain.Flight{Number: "MF8822", From: "HRB", To: "NTG"}); !ok || segment.To != "NTG" {
		t.Fatalf("直飞航班应能取到唯一航段，实际 %+v ok=%v", segment, ok)
	}
}

// TestFlightLookupMissMessage 锁住"查不到"要说清原因：日期、航段、最近可查日期。
func TestFlightLookupMissMessage(t *testing.T) {
	segments := []ResolvedFlightIdentity{
		{Number: "CZ6656", From: "TSN", To: "YIW", Date: "2026-09-15"},
		{Number: "CZ6656", From: "YIW", To: "SWA", Date: "2026-09-15"},
	}
	message := flightLookupMissMessage(
		domain.Flight{Number: "cz6656", Date: "2026-09-16", From: "tsn", To: "swa"}, segments, nil)

	for _, want := range []string{"CZ6656", "2026-09-16", "TSN→SWA", "TSN→YIW", "YIW→SWA", "最近可查日期 2026-09-15", "经停航班"} {
		if !strings.Contains(message, want) {
			t.Fatalf("提示语缺少 %q：\n%s", want, message)
		}
	}

	// 拿不到航段时退回最简说明，不要编内容
	plain := flightLookupMissMessage(domain.Flight{Number: "MF8822", Date: "2026-09-15", From: "HRB", To: "NTG"}, nil, nil)
	if plain != "EOOB 没有 MF8822 在 2026-09-15（HRB→NTG）的记录" {
		t.Fatalf("无航段信息时的说明不符预期: %s", plain)
	}
}
