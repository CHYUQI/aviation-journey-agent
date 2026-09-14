package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aviation-journey-agent/backend/internal/domain"
)

const eoobAirportFixture = `<!DOCTYPE html>
<html>
<body>
<div>
  <h5>Guangzhou Baiyun International Airport 数据概览</h5>
  <div class="row">
    <div><div><div>
      <div style="font-weight: 600; color:#999;">位置</div>
      <div><img src="/flag.svg" alt="flag">Guangzhou, 中国<br /></div>
    </div></div></div>
    <div><div><div>
      <div style="font-weight: 600; color:#999;">时区</div>
      <div>Asia/Shanghai</div>
    </div></div></div>
    <div><div><div>
      <div style="font-weight: 600; color:#999;">IATA - ICAO</div>
      <div>CAN - ZGGG</div>
    </div></div></div>
    <div><div><div>
      <div style="font-weight: 600; color:#999;">目的地</div>
      <div><a href="/CAN/mudidi">216 目的地</a></div>
    </div></div></div>
    <div><div><div>
      <div style="font-weight: 600; color:#999;">航空公司</div>
      <div>78 航空公司</div>
    </div></div></div>
    <div><div><div>
      <div style="font-weight: 600; color:#999;">航站楼</div>
      <div>3 航站楼</div>
    </div></div></div>
    <div><div><div>
      <div style="font-weight: 600; color:#999;">坐标</div>
      <div><a href="/CAN/ditu">23.387862, 113.29734</a></div>
    </div></div></div>
  </div>
  <div class="col-12">
    <div style="font-weight: 600; color:#999;">别名</div>
    <div style="margin-top: 10px;">Guangzhou Baiyun International Airport<br /></div>
  </div>
</div>
</body>
</html>`

func hourlySeats(value int) map[string]int {
	result := make(map[string]int, 24)
	for hour := 0; hour < 24; hour++ {
		result[fmt.Sprintf("%02d", hour)] = value
	}
	return result
}

func airportFixtureWithTraffic(t *testing.T) string {
	t.Helper()
	loadData := map[string]any{
		"departures": map[string]any{
			"today":    map[string]any{"general": hourlySeats(100)},
			"tomorrow": map[string]any{"general": hourlySeats(120)},
		},
		"arrivals": map[string]any{
			"today":    map[string]any{"general": hourlySeats(80)},
			"tomorrow": map[string]any{"general": hourlySeats(90)},
		},
	}
	raw, err := json.Marshal(loadData)
	if err != nil {
		t.Fatal(err)
	}
	script := "<script>window.airportLoadData = " + string(raw) + ";</script>\n</body>"
	return strings.Replace(eoobAirportFixture, "</body>", script, 1)
}

func TestParseEOOBAirportInfo(t *testing.T) {
	lines, err := parseEOOBAirportInfo(eoobAirportFixture)
	if err != nil {
		t.Fatalf("parseEOOBAirportInfo 返回错误: %v", err)
	}

	want := []string{
		"机场：Guangzhou Baiyun International Airport（CAN - ZGGG）",
		"位置：Guangzhou, 中国；时区：Asia/Shanghai",
		"航站楼：3 个",
		"可飞往目的地：216 个",
		"航司数量：78 家",
		"坐标：23.387862, 113.29734",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("解析结果不符\n got: %#v\nwant: %#v", lines, want)
	}
}

func TestParseEOOBAirportInfo_MissingOverview(t *testing.T) {
	_, err := parseEOOBAirportInfo(`<html><body><h1>not an airport page</h1></body></html>`)
	if err == nil {
		t.Fatal("没有数据概览时应返回错误")
	}
}

func TestAirportStatusSkillExecuteHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/CAN":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(airportFixtureWithTraffic(t)))
		case "/api/delaybox/CAN":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"delayindex_observations":100,"delayindex_datestart":"2026-09-14 01:00:00","delayindex_dateend":"2026-09-14 04:00:00","delayindex_flights":100,"onTime":90,"delayed15":8,"delayed30":2,"delayed45":0,"canceled":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	skill := AirportStatusSkill{Client: server.Client(), BaseURL: server.URL}
	result, err := skill.Execute(context.Background(), Query{Journey: domain.Journey{
		Flights: []domain.Flight{{Number: "CA1301", Date: "2026-09-14", From: "CAN", To: "PEK"}},
	}})
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if len(result.Issues) != 0 {
		t.Fatalf("Issues = %#v，期望为空", result.Issues)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("Observations = %d 条，期望 1 条", len(result.Observations))
	}

	obs := result.Observations[0]
	if obs.Field != FieldAirportGuide {
		t.Fatalf("Field = %q，期望 %q", obs.Field, FieldAirportGuide)
	}
	if obs.Source != "EOOB 机场页/延误接口" || obs.Confidence != "high" || obs.ObservedAt == "" {
		t.Fatalf("观测值元数据不完整: %#v", obs)
	}
	lines, ok := obs.Value.([]string)
	if !ok || len(lines) != 10 {
		t.Fatalf("guide 值 = %#v，期望 10 行字符串", obs.Value)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"客流（出发）", "客流（到达）", "机场延误情况（最近统计窗口）：共", "平均延误（按延误档位下限估算）"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("guide 缺少 %q:\n%s", want, joined)
		}
	}
}

func TestAirportStatusSkillExecute_InvalidIATA(t *testing.T) {
	result, err := NewAirportStatusSkill().Execute(context.Background(), Query{Journey: domain.Journey{
		Flights: []domain.Flight{{Number: "CA1301", Date: "2026-09-14", From: "CANADA", To: "PEK"}},
	}})
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if len(result.Observations) != 0 || len(result.Issues) == 0 {
		t.Fatalf("非法三字码应返回 issue，实际: %#v", result)
	}
}

func TestExtractJSONObject(t *testing.T) {
	raw := `window.airportLoadData = {"a":{"b":1},"c":"}"};`
	got, err := extractJSONObject(raw, "window.airportLoadData")
	if err != nil {
		t.Fatalf("extractJSONObject 返回错误: %v", err)
	}
	if got != `{"a":{"b":1},"c":"}"}` {
		t.Fatalf("extractJSONObject = %q", got)
	}
}

func TestAirportTrafficLines(t *testing.T) {
	loadData := eoobAirportLoadData{
		Departures: eoobAirportLoadDirection{
			Today:    map[string]map[string]int{"general": {"08": 100, "09": 110}},
			Tomorrow: map[string]map[string]int{"general": {"07": 90}},
		},
		Arrivals: eoobAirportLoadDirection{
			Today:    map[string]map[string]int{"general": {"08": 80, "09": 70}},
			Tomorrow: map[string]map[string]int{"general": {"07": 60}},
		},
	}
	lines := airportTrafficLines(loadData, 8)
	if len(lines) != 2 {
		t.Fatalf("客流行数 = %d，期望 2", len(lines))
	}
	if !strings.Contains(lines[0], "当前小时计划旅客座位数 100") || !strings.Contains(lines[0], "未来24小时约 300") {
		t.Fatalf("出发客流行不符合预期: %s", lines[0])
	}
	if !strings.Contains(lines[1], "当前小时计划旅客座位数 80") || !strings.Contains(lines[1], "未来24小时约 210") {
		t.Fatalf("到达客流行不符合预期: %s", lines[1])
	}
}

func TestFormatAirportTrafficLine_CurrentMissing(t *testing.T) {
	line := formatAirportTrafficLine("出发", 0, 100)
	if !strings.Contains(line, "当前小时数据暂缺") || !strings.Contains(line, "未来24小时约 100") {
		t.Fatalf("当前小时无数据时的客流提示不符合预期: %s", line)
	}
}

func TestFormatEOOBDelayInfo(t *testing.T) {
	lines := formatEOOBDelayInfo(eoobDelayBox{
		Observations: 100,
		DateStart:    "2026-09-14 01:00:00",
		DateEnd:      "2026-09-14 04:00:00",
		Flights:      100,
		OnTime:       90,
		Delayed15:    8,
		Delayed30:    2,
	})
	if len(lines) != 2 {
		t.Fatalf("延误行数 = %d，期望 2", len(lines))
	}
	if !strings.Contains(lines[0], "机场延误情况（最近统计窗口）：共 100 个航班") || !strings.Contains(lines[0], "09-14 01:00") {
		t.Fatalf("延误统计行不符合预期: %s", lines[0])
	}
	if !strings.Contains(lines[1], "1.8 分钟") || !strings.Contains(lines[1], "18.0 分钟") {
		t.Fatalf("延误均值行不符合预期: %s", lines[1])
	}
}
