package data

import "testing"

func TestVerifyQuote(t *testing.T) {
	page := `<html><body>
		<div class="eta">预计 <b>47 分钟</b> 到达，当前路况拥堵</div>
		<div>更新时间&nbsp;12:35</div>
	</body></html>`

	cases := []struct {
		name  string
		quote string
		want  bool
	}{
		{"原文命中（跨标签）", "预计 47 分钟 到达，当前路况拥堵", true},
		{"原文命中（含不换行空格）", "更新时间 12:35", true},
		{"页面里没有这句话", "预计 25 分钟到达，路况畅通", false},
		{"模型编的数字", "预计 18 分钟到达，当前路况拥堵", false},
		{"摘录太短没有验证价值", "47", false},
		{"空摘录", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := VerifyQuote(page, tc.quote); got != tc.want {
				t.Fatalf("VerifyQuote(page, %q) = %v, want %v", tc.quote, got, tc.want)
			}
		})
	}
}
