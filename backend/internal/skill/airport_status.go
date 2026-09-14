package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	xhtml "golang.org/x/net/html"

	"aviation-journey-agent/backend/internal/browser"
)

const (
	eoobAirportHTTPTimeout = 20 * time.Second
	eoobAirportMaxHTMLSize = 2 << 20
	eoobAirportMaxJSONSize = 1 << 20
	eoobAirportWait        = 25 * time.Second
	// eoobAirportUserAgent 必须跟当前浏览器版本同代。
	// 实测 Chrome/120 会被 Cloudflare 403；Chrome/153 能直接拿到静态 HTML。
	eoobAirportUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36"
)

// AirportStatusSkill 查询机场基础信息、计划客流和近期延误情况。
//
// URL 模式：
//   - 机场页 HTML：{base}/{IATA}，解析数据概览和内嵌客流；
//   - 延误接口：{base}/api/delaybox/{IATA}，页面自己的延误组件使用同一接口。
//
// 字段结构固定，不使用模型抽取；HTTP 被 Cloudflare 拦时用浏览器兜底。
type AirportStatusSkill struct {
	Client  *http.Client
	BaseURL string
}

func NewAirportStatusSkill() AirportStatusSkill {
	return AirportStatusSkill{
		Client:  &http.Client{Timeout: eoobAirportHTTPTimeout},
		BaseURL: eoobBase,
	}
}

func (AirportStatusSkill) Name() string { return "airport_status" }

func (AirportStatusSkill) Supports(field string) bool { return field == FieldAirportGuide }

func (s AirportStatusSkill) Execute(ctx context.Context, query Query) (Result, error) {
	if len(query.Journey.Flights) == 0 {
		return Result{Issues: []string{"行程里没有航段，无法查询机场信息"}}, nil
	}

	iata := strings.ToUpper(strings.TrimSpace(query.Journey.Flights[0].From))
	if !validAirportIATA(iata) {
		return Result{Issues: []string{fmt.Sprintf("出发机场代码 %q 不是三字码，无法查询机场信息", iata)}}, nil
	}

	pageURL := s.baseURL() + "/" + iata
	rawHTML, err := s.fetchAirportHTML(ctx, pageURL)
	if err != nil {
		return Result{Issues: []string{fmt.Sprintf("EOOB 机场页查询失败：%v", err)}}, nil
	}

	overview, err := parseEOOBAirportOverview(rawHTML)
	if err != nil {
		return Result{Issues: []string{fmt.Sprintf("解析 EOOB 机场页失败：%v", err)}}, nil
	}
	if timezone := strings.TrimSpace(overview["时区"]); timezone != "" {
		cacheEOOBAirportTimezone(iata, timezone)
	}
	if lat, lng, coordinateErr := parseEOOBAirportCoordinates(overview["坐标"]); coordinateErr == nil {
		cacheEOOBAirportCoordinates(iata, lat, lng)
	}

	lines := formatEOOBAirportInfo(overview)
	issues := []string{}

	if traffic, trafficErr := parseEOOBAirportTraffic(rawHTML, overview["时区"]); trafficErr != nil {
		issues = append(issues, "EOOB 机场客流解析失败："+trafficErr.Error())
	} else {
		lines = append(lines, traffic...)
	}

	if delay, delayErr := s.fetchEOOBDelay(ctx, iata); delayErr != nil {
		issues = append(issues, "EOOB 机场延误数据获取失败："+delayErr.Error())
	} else {
		lines = append(lines, formatEOOBDelayInfo(*delay)...)
	}

	return Result{
		Observations: []Observation{{
			Field:      FieldAirportGuide,
			Value:      lines,
			Source:     "EOOB 机场页/延误接口",
			ObservedAt: time.Now().Format(time.RFC3339),
			Confidence: "high",
		}},
		Issues: issues,
	}, nil
}

func validAirportIATA(code string) bool {
	if len(code) != 3 {
		return false
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func (s AirportStatusSkill) baseURL() string {
	baseURL := strings.TrimRight(strings.TrimSpace(s.BaseURL), "/")
	if baseURL == "" {
		baseURL = eoobBase
	}
	return baseURL
}

type cachedEOOBCoordinates struct {
	lat     float64
	lng     float64
	expires time.Time
}

var eoobAirportCoordinateCache sync.Map

func cachedEOOBAirportCoordinates(iata string) (float64, float64, bool) {
	value, ok := eoobAirportCoordinateCache.Load(strings.ToUpper(strings.TrimSpace(iata)))
	if !ok {
		return 0, 0, false
	}
	cached, ok := value.(cachedEOOBCoordinates)
	if !ok || time.Now().After(cached.expires) {
		eoobAirportCoordinateCache.Delete(strings.ToUpper(strings.TrimSpace(iata)))
		return 0, 0, false
	}
	return cached.lat, cached.lng, true
}

func cacheEOOBAirportCoordinates(iata string, lat, lng float64) {
	eoobAirportCoordinateCache.Store(strings.ToUpper(strings.TrimSpace(iata)), cachedEOOBCoordinates{
		lat:     lat,
		lng:     lng,
		expires: time.Now().Add(eoobResolutionCacheTTL),
	})
}

func parseEOOBAirportCoordinates(raw string) (float64, float64, error) {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("坐标格式不正确: %q", raw)
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("纬度解析失败: %w", err)
	}
	lng, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("经度解析失败: %w", err)
	}
	return lat, lng, nil
}

// fetchEOOBAirportOverview 让依赖机场页的 Skill 自己走 URL 解析，
// 不要求 AirportStatusSkill 必须先在 Registry 里执行。
func fetchEOOBAirportOverview(ctx context.Context, iata string) (map[string]string, error) {
	iata = strings.ToUpper(strings.TrimSpace(iata))
	if !validAirportIATA(iata) {
		return nil, fmt.Errorf("机场代码 %q 不合法", iata)
	}

	skill := NewAirportStatusSkill()
	rawHTML, err := skill.fetchAirportHTML(ctx, skill.baseURL()+"/"+iata)
	if err != nil {
		return nil, err
	}
	overview, err := parseEOOBAirportOverview(rawHTML)
	if err != nil {
		return nil, err
	}
	if timezone := strings.TrimSpace(overview["时区"]); timezone != "" {
		cacheEOOBAirportTimezone(iata, timezone)
	}
	if lat, lng, coordinateErr := parseEOOBAirportCoordinates(overview["坐标"]); coordinateErr == nil {
		cacheEOOBAirportCoordinates(iata, lat, lng)
	}
	return overview, nil
}

func resolveEOOBAirportTimezone(ctx context.Context, iata string) (string, error) {
	iata = strings.ToUpper(strings.TrimSpace(iata))
	if timezone, ok := cachedEOOBAirportTimezone(iata); ok {
		return timezone, nil
	}
	overview, err := fetchEOOBAirportOverview(ctx, iata)
	if err != nil {
		return "", err
	}
	timezone := strings.TrimSpace(overview["时区"])
	if timezone == "" {
		return "", errors.New("机场页没有时区")
	}
	return timezone, nil
}

func resolveEOOBAirportCoordinates(ctx context.Context, iata string) (float64, float64, error) {
	iata = strings.ToUpper(strings.TrimSpace(iata))
	if lat, lng, ok := cachedEOOBAirportCoordinates(iata); ok {
		return lat, lng, nil
	}
	overview, err := fetchEOOBAirportOverview(ctx, iata)
	if err != nil {
		return 0, 0, err
	}
	lat, lng, err := parseEOOBAirportCoordinates(overview["坐标"])
	if err != nil {
		return 0, 0, err
	}
	return lat, lng, nil
}

func (s AirportStatusSkill) fetchAirportHTML(ctx context.Context, pageURL string) (string, error) {
	body, httpErr := s.fetchAirportHTMLHTTP(ctx, pageURL)
	if httpErr == nil {
		return body, nil
	}

	body, browserErr := fetchAirportHTMLBrowser(ctx, pageURL)
	if browserErr == nil {
		return body, nil
	}

	return "", fmt.Errorf("HTTP 访问失败（%v），浏览器兜底也失败（%v）", httpErr, browserErr)
}

func (s AirportStatusSkill) fetchAirportHTMLHTTP(ctx context.Context, pageURL string) (string, error) {
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: eoobAirportHTTPTimeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	setEOOBHeaders(req, "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, eoobAirportMaxHTMLSize))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	text := string(body)
	if isCloudflareChallenge(text) {
		return "", errors.New("被 Cloudflare 挑战页拦截")
	}
	if !strings.Contains(text, "数据概览") {
		return "", errors.New("页面里没有找到数据概览")
	}
	return text, nil
}

func fetchAirportHTMLBrowser(ctx context.Context, pageURL string) (string, error) {
	b, err := browser.Launch(ctx, browser.LaunchOptions{})
	if err != nil {
		return "", err
	}
	defer func() { _ = b.Close() }()

	if err := b.Navigate(pageURL); err != nil {
		return "", err
	}
	if err := b.WaitText("数据概览", eoobAirportWait); err != nil {
		return "", err
	}

	rawHTML, err := b.EvaluateString("document.documentElement.outerHTML")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(rawHTML) == "" {
		return "", errors.New("浏览器没有返回 HTML")
	}
	return rawHTML, nil
}

func (s AirportStatusSkill) fetchEOOBDelay(ctx context.Context, iata string) (*eoobDelayBox, error) {
	apiURL := s.baseURL() + "/api/delaybox/" + iata

	body, httpErr := s.fetchEOOBDelayHTTP(ctx, apiURL)
	if httpErr == nil {
		return parseEOOBDelayBox(body)
	}

	body, browserErr := fetchEOOBDelayBrowser(ctx, s.baseURL()+"/"+iata, apiURL)
	if browserErr == nil {
		return parseEOOBDelayBox(body)
	}

	return nil, fmt.Errorf("HTTP 访问失败（%v），浏览器兜底也失败（%v）", httpErr, browserErr)
}

func (s AirportStatusSkill) fetchEOOBDelayHTTP(ctx context.Context, apiURL string) ([]byte, error) {
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: eoobAirportHTTPTimeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	setEOOBHeaders(req, "application/json, text/plain, */*")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, eoobAirportMaxJSONSize))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if isCloudflareChallenge(string(body)) {
		return nil, errors.New("被 Cloudflare 挑战页拦截")
	}
	return body, nil
}

func fetchEOOBDelayBrowser(ctx context.Context, pageURL, apiURL string) ([]byte, error) {
	b, err := browser.Launch(ctx, browser.LaunchOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = b.Close() }()

	if err := b.Navigate(pageURL); err != nil {
		return nil, err
	}
	if err := b.WaitText("数据概览", eoobAirportWait); err != nil {
		return nil, err
	}

	quotedURL, _ := json.Marshal(apiURL)
	expr := fmt.Sprintf(`(async () => {
		const resp = await fetch(%s, { headers: { 'Accept': 'application/json, text/plain, */*' } })
		if (!resp.ok) return '__HTTP_' + resp.status
		return await resp.text()
	})()`, quotedURL)

	raw, err := b.EvaluateString(expr)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(raw, "__HTTP_") {
		return nil, errors.New(raw)
	}
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("延误接口返回空内容")
	}
	if isCloudflareChallenge(raw) {
		return nil, errors.New("延误接口被 Cloudflare 挑战页拦截")
	}
	return []byte(raw), nil
}

func setEOOBHeaders(req *http.Request, accept string) {
	req.Header.Set("User-Agent", eoobAirportUserAgent)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
}

func isCloudflareChallenge(text string) bool {
	return strings.Contains(text, "Just a moment") ||
		strings.Contains(text, "cf-chl") ||
		strings.Contains(text, "challenge-error-text")
}

var eoobAirportOverviewLabels = []string{
	"位置",
	"时区",
	"IATA - ICAO",
	"目的地",
	"航空公司",
	"航站楼",
	"坐标",
	"别名",
}

func parseEOOBAirportInfo(rawHTML string) ([]string, error) {
	values, err := parseEOOBAirportOverview(rawHTML)
	if err != nil {
		return nil, err
	}
	return formatEOOBAirportInfo(values), nil
}

func parseEOOBAirportOverview(rawHTML string) (map[string]string, error) {
	doc, err := xhtml.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return nil, err
	}

	heading := findHTMLElement(doc, func(n *xhtml.Node) bool {
		return n.Data == "h5" && strings.Contains(htmlNodeText(n), "数据概览")
	})
	if heading == nil || heading.Parent == nil {
		return nil, errors.New("没有找到数据概览")
	}

	values := make(map[string]string, len(eoobAirportOverviewLabels))
	for _, label := range eoobAirportOverviewLabels {
		labelNode := findHTMLElement(heading.Parent, func(n *xhtml.Node) bool {
			return htmlDirectText(n) == label
		})
		if labelNode == nil {
			continue
		}
		if value := htmlNextValue(labelNode); value != "" {
			values[label] = value
		}
	}
	if len(values) == 0 {
		return nil, errors.New("数据概览里没有可解析的字段")
	}
	return values, nil
}

func findHTMLElement(n *xhtml.Node, match func(*xhtml.Node) bool) *xhtml.Node {
	if n == nil {
		return nil
	}
	if n.Type == xhtml.ElementNode && match(n) {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if found := findHTMLElement(child, match); found != nil {
			return found
		}
	}
	return nil
}

func htmlDirectText(n *xhtml.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.TextNode {
			b.WriteString(child.Data)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func htmlNodeText(n *xhtml.Node) string {
	if n == nil {
		return ""
	}
	var parts []string
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.TextNode {
			if text := strings.Join(strings.Fields(node.Data), " "); text != "" {
				parts = append(parts, text)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return strings.Join(parts, "、")
}

func htmlNextValue(label *xhtml.Node) string {
	for sibling := label.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		if sibling.Type != xhtml.ElementNode {
			continue
		}
		if value := htmlNodeText(sibling); value != "" {
			return value
		}
	}
	return ""
}

func formatEOOBAirportInfo(values map[string]string) []string {
	lines := make([]string, 0, 8)

	if alias := values["别名"]; alias != "" {
		line := "机场：" + alias
		if code := values["IATA - ICAO"]; code != "" {
			line += "（" + code + "）"
		}
		lines = append(lines, line)
	} else if code := values["IATA - ICAO"]; code != "" {
		lines = append(lines, "机场代码："+code)
	}

	if location := values["位置"]; location != "" {
		line := "位置：" + location
		if zone := values["时区"]; zone != "" {
			line += "；时区：" + zone
		}
		lines = append(lines, line)
	} else if zone := values["时区"]; zone != "" {
		lines = append(lines, "时区："+zone)
	}

	if terminals := strings.TrimSpace(strings.TrimSuffix(values["航站楼"], "航站楼")); terminals != "" {
		lines = append(lines, "航站楼："+terminals+" 个")
	}
	if destinations := strings.TrimSpace(strings.TrimSuffix(values["目的地"], "目的地")); destinations != "" {
		lines = append(lines, "可飞往目的地："+destinations+" 个")
	}
	if airlines := strings.TrimSpace(strings.TrimSuffix(values["航空公司"], "航空公司")); airlines != "" {
		lines = append(lines, "航司数量："+airlines+" 家")
	}
	if coordinates := values["坐标"]; coordinates != "" {
		lines = append(lines, "坐标："+coordinates)
	}
	return lines
}

type eoobAirportLoadData struct {
	Departures eoobAirportLoadDirection `json:"departures"`
	Arrivals   eoobAirportLoadDirection `json:"arrivals"`
}

type eoobAirportLoadDirection struct {
	Today    map[string]map[string]int `json:"today"`
	Tomorrow map[string]map[string]int `json:"tomorrow"`
}

func parseEOOBAirportTraffic(rawHTML, timezone string) ([]string, error) {
	jsonText, err := extractJSONObject(rawHTML, "window.airportLoadData")
	if err != nil {
		return nil, err
	}

	var loadData eoobAirportLoadData
	if err := json.Unmarshal([]byte(jsonText), &loadData); err != nil {
		return nil, fmt.Errorf("解析 window.airportLoadData 失败: %w", err)
	}
	if strings.TrimSpace(timezone) == "" {
		return nil, errors.New("页面没有时区，无法判断当前客流时段")
	}

	loc, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil {
		return nil, fmt.Errorf("无法识别机场时区 %q: %w", timezone, err)
	}
	lines := airportTrafficLines(loadData, time.Now().In(loc).Hour())
	if len(lines) == 0 {
		return nil, errors.New("页面没有可用的客流数据")
	}
	return lines, nil
}

func airportTrafficLines(loadData eoobAirportLoadData, currentHour int) []string {
	departuresNow := rollingSeatCount(loadData.Departures, currentHour, 0)
	arrivalsNow := rollingSeatCount(loadData.Arrivals, currentHour, 0)
	departures24h := rollingSeatTotal(loadData.Departures, currentHour)
	arrivals24h := rollingSeatTotal(loadData.Arrivals, currentHour)
	if departures24h == 0 && arrivals24h == 0 {
		return nil
	}
	return []string{
		formatAirportTrafficLine("出发", departuresNow, departures24h),
		formatAirportTrafficLine("到达", arrivalsNow, arrivals24h),
	}
}

func formatAirportTrafficLine(label string, current, total int) string {
	if current > 0 {
		return fmt.Sprintf("客流（%s）：当前小时计划旅客座位数 %d，未来24小时约 %d", label, current, total)
	}
	return fmt.Sprintf("客流（%s）：当前小时数据暂缺，未来24小时约 %d", label, total)
}

func rollingSeatTotal(direction eoobAirportLoadDirection, currentHour int) int {
	total := 0
	for offset := 0; offset < 24; offset++ {
		total += rollingSeatCount(direction, currentHour, offset)
	}
	return total
}

func rollingSeatCount(direction eoobAirportLoadDirection, currentHour, offset int) int {
	hour := currentHour + offset
	key := fmt.Sprintf("%02d", hour%24)
	if hour < 24 {
		return direction.Today["general"][key]
	}
	return direction.Tomorrow["general"][key]
}

func extractJSONObject(rawText, marker string) (string, error) {
	markerIndex := strings.Index(rawText, marker)
	if markerIndex < 0 {
		return "", fmt.Errorf("页面里没有找到 %s", marker)
	}
	start := strings.Index(rawText[markerIndex+len(marker):], "{")
	if start < 0 {
		return "", fmt.Errorf("%s 后面没有 JSON 对象", marker)
	}
	start += markerIndex + len(marker)

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(rawText); i++ {
		c := rawText[i]
		if inString {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return rawText[start : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("%s 的 JSON 对象不完整", marker)
}

type eoobDelayBox struct {
	Observations int    `json:"delayindex_observations"`
	DateStart    string `json:"delayindex_datestart"`
	DateEnd      string `json:"delayindex_dateend"`
	Flights      int    `json:"delayindex_flights"`
	OnTime       int    `json:"onTime"`
	Delayed15    int    `json:"delayed15"`
	Delayed30    int    `json:"delayed30"`
	Delayed45    int    `json:"delayed45"`
	Canceled     int    `json:"canceled"`
}

func parseEOOBDelayBox(body []byte) (*eoobDelayBox, error) {
	var box eoobDelayBox
	if err := json.Unmarshal(body, &box); err != nil {
		return nil, fmt.Errorf("解析延误数据失败: %w", err)
	}
	if box.Flights == 0 && box.Observations == 0 &&
		box.OnTime == 0 && box.Delayed15 == 0 && box.Delayed30 == 0 &&
		box.Delayed45 == 0 && box.Canceled == 0 {
		return nil, errors.New("延误接口没有返回统计")
	}
	return &box, nil
}

func formatEOOBDelayInfo(box eoobDelayBox) []string {
	lines := make([]string, 0, 2)
	window := formatEOOBDelayWindow(box)
	lines = append(lines, fmt.Sprintf(
		"机场延误情况（最近统计窗口）：共 %d 个航班，其中准点 %d，延误15分钟 %d，延误30分钟 %d，延误45分钟以上 %d，取消 %d%s",
		box.Flights, box.OnTime, box.Delayed15, box.Delayed30, box.Delayed45, box.Canceled, window))

	if box.Flights <= 0 {
		return lines
	}

	delayed := box.Delayed15 + box.Delayed30 + box.Delayed45
	lowerBoundMinutes := box.Delayed15*15 + box.Delayed30*30 + box.Delayed45*45
	averageAll := float64(lowerBoundMinutes) / float64(box.Flights)
	if delayed == 0 {
		lines = append(lines, fmt.Sprintf("平均延误（按延误档位下限估算）：全部航班约 %.1f 分钟", averageAll))
		return lines
	}

	averageDelayed := float64(lowerBoundMinutes) / float64(delayed)
	lines = append(lines, fmt.Sprintf(
		"平均延误（按延误档位下限估算）：全部航班约 %.1f 分钟；延误航班约 %.1f 分钟",
		averageAll, averageDelayed))
	return lines
}

func formatEOOBDelayWindow(box eoobDelayBox) string {
	start := formatEOOBDelayTime(box.DateStart)
	end := formatEOOBDelayTime(box.DateEnd)
	if start == "" || end == "" {
		return ""
	}
	return fmt.Sprintf("（%s 至 %s UTC）", start, end)
}

func formatEOOBDelayTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", raw, time.UTC)
	if err != nil {
		return raw
	}
	return parsed.Format("01-02 15:04")
}
