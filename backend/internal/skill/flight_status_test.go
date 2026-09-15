package skill

import "testing"

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
