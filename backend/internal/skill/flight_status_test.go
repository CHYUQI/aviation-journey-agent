package skill

import "testing"

func TestNormalizeTime(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		date string
		want string
	}{
		{"完整时间直接用", "2026-09-13T13:05:00+08:00", "2026-09-13", "2026-09-13T13:05:00+08:00"},
		{"只有钟点补日期时区", "13:05", "2026-09-13", "2026-09-13T13:05:00+08:00"},
		{"官网的月日写法", "09月13日 08:00", "2026-09-13", "2026-09-13T08:00:00+08:00"},
		{"带秒的钟点", "16:15:30", "2026-09-13", "2026-09-13T16:15:30+08:00"},
		{"空字符串", "", "2026-09-13", ""},
		{"无法解析的写法一律丢弃", "下午一点", "2026-09-13", ""},
		{"日期非法则不拼", "13:05", "不是日期", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeTime(tc.raw, tc.date); got != tc.want {
				t.Fatalf("normalizeTime(%q, %q) = %q, want %q", tc.raw, tc.date, got, tc.want)
			}
		})
	}
}

func TestMapStatusText(t *testing.T) {
	cases := map[string]string{
		"正常":   "on_time",
		"正点":   "on_time",
		"延误":   "delayed",
		"取消":   "cancelled",
		"已起飞":  "departed",
		"我不确定": "unknown",
		"":     "unknown",
	}
	for raw, want := range cases {
		if got := mapStatusText(raw); got != want {
			t.Fatalf("mapStatusText(%q) = %q, want %q", raw, got, want)
		}
	}
}
