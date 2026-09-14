package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aviation-journey-agent/backend/internal/browser"
	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/model"
	"aviation-journey-agent/backend/internal/textutil"
)

// eoobBase 是聚合数据源。它一个站覆盖所有航司与机场，
// 航班页地址可以直接由「航班号-出发-到达」拼出来，不需要模拟搜索。
const (
	eoobBase        = "https://www.eoob.com.cn"
	eoobStatusBase  = "https://apil.eoob.com"
	eoobStatusLimit = 1 << 20
)

// FlightTimes 是航班的关键时间节点，用于生成 State.Timeline。
type FlightTimes struct {
	DepartureTime string // RFC3339
	BoardingTime  string // RFC3339
	GateCloseTime string // RFC3339
}

// pageSource 是一种获取页面内容的方式。
type pageSource struct {
	name  string
	fetch func() (pageContent, error)
}

// pageContent 是一次抓取的结果。
type pageContent struct {
	text string
	// departureHint 是用页面结构直接读出来的计划起飞时间（如 "08:00"）。
	//
	// 为什么需要它：看板卡片里除了计划时间，还有一个「看板更新时间」
	// （实测是 UTC 时刻），两个时间长得一模一样。实测让模型抽，
	// 换了两种提示词都会取错。这种「同一块文本里有两个时间」的情况，
	// 用固定规则读才可靠。
	//
	// 读不到时留空，退回让模型抽 —— 页面改版时不至于整条链路失效。
	structured *structuredFields
}

// structuredFields 是从页面固定 id 直接读出来的航班字段。
// 为 nil 表示这家页面没有稳定锚点，退回让模型抽。
type structuredFields struct {
	StatusText    string // 状态原话，如「计划」
	Gate          string // 登机口
	DepartureTime string // 原始写法，如 "08:00"
}

// isZero 表示什么都没读到。
func (f structuredFields) isZero() bool {
	return f.StatusText == "" && f.Gate == "" && f.DepartureTime == ""
}

// FlightStatusSkill 查询航班动态：状态、登机口、计划起飞时间。
//
// 取数策略是「主源 + 兜底」：
//
//	主源   EOOB（eoob.com.cn）：一个站覆盖所有航司，URL 可直接构造，
//	       页面里还有历史准点记录和登机口记录
//	兜底   航司官网：只覆盖已实测通过配方的航司，用来在主源缺数据时补
//
// 两条路拿到的都是页面纯文本，后面的抽取与校验完全一样：
// 模型只负责抄原文，代码把抽出来的值回原文核对，找不到就丢弃。
type FlightStatusSkill struct {
	client     model.Client
	HTTPClient *http.Client
}

func NewFlightStatusSkill(client model.Client) *FlightStatusSkill {
	return &FlightStatusSkill{
		client:     client,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *FlightStatusSkill) Name() string { return "flight_status" }

func (s *FlightStatusSkill) Supports(field string) bool {
	switch field {
	case FieldFlightStatus, FieldFlightTimes, FieldFlightGate:
		return true
	default:
		return false
	}
}

func (s *FlightStatusSkill) Execute(ctx context.Context, query Query) (Result, error) {
	if len(query.Journey.Flights) == 0 {
		return Result{Issues: []string{"行程里没有航段，无法查询航班动态"}}, nil
	}
	flight := query.Journey.Flights[0]

	// 快速路径：页面自己的航班状态接口。日期和航线由 FlightIdentitySkill 解析后传入。
	if result, ok := s.fetchEOOBStatusAPI(ctx, flight); ok {
		return result, nil
	}

	if s.client == nil {
		return Result{Issues: []string{"未配置模型，无法解析航班页面内容"}}, nil
	}

	sources := []pageSource{
		{name: "EOOB", fetch: func() (pageContent, error) { return s.fetchEOOB(ctx, flight) }},
	}
	if _, ok := RecipeFor(flight.Number); ok {
		sources = append(sources, pageSource{
			name: "航司官网", fetch: func() (pageContent, error) { return s.fetchAirline(ctx, flight) },
		})
	}

	issues := make([]string, 0, len(sources))
	for _, src := range sources {
		page, err := src.fetch()
		if err != nil {
			issues = append(issues, fmt.Sprintf("%s 查询失败：%v", src.name, err))
			continue
		}

		result, err := s.extract(ctx, page, src.name, flight)
		if err != nil {
			issues = append(issues, fmt.Sprintf("解析 %s 页面失败：%v", src.name, err))
			continue
		}
		if len(result.Observations) > 0 {
			if len(issues) > 0 {
				// 有源失败但兜底成功了，把原因记到日志里，别让问题无声无息
				log.Printf("[flight_status] %s 没拿到，已由后续数据源兜底", strings.Join(issues, "；"))
			}
			return result, nil
		}
		issues = append(issues, result.Issues...)
	}

	if len(issues) == 0 {
		issues = append(issues, "没有查到该航班的动态")
	}
	return Result{Issues: issues}, nil
}

type eoobStatusPayload struct {
	Schedule *struct {
		Matched       bool   `json:"matched"`
		Status        string `json:"status"`
		DepartureUTC  int64  `json:"departure_utc"`
		ArrivalUTC    int64  `json:"arrival_utc"`
		DepartureDate string `json:"departure_date"`
		DepartureTime string `json:"departure_time"`
	} `json:"schedule"`
	FIDS struct {
		Departure *struct {
			Matched     bool   `json:"matched"`
			Gate        string `json:"gate"`
			RemarksCode string `json:"remarks_code"`
			RemarksText string `json:"remarks_text"`
		} `json:"departure"`
	} `json:"fids"`
}

func (s *FlightStatusSkill) fetchEOOBStatusAPI(ctx context.Context, flight domain.Flight) (Result, bool) {
	number := strings.ToUpper(strings.TrimSpace(flight.Number))
	from := strings.ToUpper(strings.TrimSpace(flight.From))
	to := strings.ToUpper(strings.TrimSpace(flight.To))
	date := strings.TrimSpace(flight.Date)
	if number == "" || from == "" || to == "" || date == "" {
		return Result{}, false
	}

	endpoint := eoobStatusBase + "/api/flight/status/" + url.PathEscape(number) + ".json"
	params := url.Values{}
	params.Set("with", "fids")
	params.Set("from", from)
	params.Set("to", to)
	params.Set("departure_date", date)
	params.Set("lang", "zh")
	endpoint += "?" + params.Encode()

	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Result{}, false
	}
	req.Header.Set("User-Agent", eoobAirportUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", eoobBase)
	req.Header.Set("Referer", eoobBase+"/"+number+"-"+from+"-"+to)

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, eoobStatusLimit))
	if err != nil || resp.StatusCode != http.StatusOK {
		return Result{}, false
	}

	var payload eoobStatusPayload
	if err := json.Unmarshal(body, &payload); err != nil || payload.Schedule == nil || !payload.Schedule.Matched {
		return Result{}, false
	}

	observedAt := time.Now().Format(time.RFC3339)
	observations := make([]Observation, 0, 3)

	statusCode := payload.Schedule.Status
	if payload.FIDS.Departure != nil && payload.FIDS.Departure.Matched && payload.FIDS.Departure.RemarksCode != "" {
		statusCode = payload.FIDS.Departure.RemarksCode
	}
	if status := mapEOOBStatusCode(statusCode); status != domain.FlightStatusUnknown {
		observations = append(observations, Observation{
			Field: FieldFlightStatus, Value: status,
			Source: "EOOB 航班状态接口", ObservedAt: observedAt, Confidence: "high",
		})
	}

	if payload.FIDS.Departure != nil && payload.FIDS.Departure.Matched {
		if gate := strings.TrimSpace(payload.FIDS.Departure.Gate); gate != "" {
			observations = append(observations, Observation{
				Field: FieldFlightGate, Value: gate,
				Source: "EOOB 航班状态接口", ObservedAt: observedAt, Confidence: "high",
			})
		}
	}

	location := time.UTC
	timezoneIssues := []string{}
	if timezone, timezoneErr := resolveEOOBAirportTimezone(ctx, from); timezoneErr == nil {
		if resolved, loadErr := time.LoadLocation(timezone); loadErr == nil {
			location = resolved
		} else {
			timezoneIssues = append(timezoneIssues, "出发机场时区无法识别："+loadErr.Error())
		}
	} else {
		timezoneIssues = append(timezoneIssues, "出发机场时区解析失败："+timezoneErr.Error())
	}

	times := FlightTimes{}
	if payload.Schedule.DepartureUTC > 0 {
		times.DepartureTime = time.Unix(payload.Schedule.DepartureUTC, 0).In(location).Format(time.RFC3339)
	} else if payload.Schedule.DepartureTime != "" && payload.Schedule.DepartureDate != "" {
		times.DepartureTime = normalizeTimeInLocation(payload.Schedule.DepartureTime, payload.Schedule.DepartureDate, location)
	}
	if times.DepartureTime != "" {
		observations = append(observations, Observation{
			Field: FieldFlightTimes, Value: times,
			Source: "EOOB 航班状态接口", ObservedAt: observedAt, Confidence: "high",
		})
	}

	if len(observations) == 0 {
		return Result{}, false
	}
	return Result{Observations: observations, Issues: timezoneIssues}, true
}

func mapEOOBStatusCode(code string) string {
	value := strings.ToLower(strings.TrimSpace(code))
	switch {
	case value == "":
		return domain.FlightStatusUnknown
	case strings.Contains(value, "cancel"):
		return domain.FlightStatusCancelled
	case strings.Contains(value, "divert"):
		return domain.FlightStatusDiverted
	case strings.Contains(value, "departed"), strings.Contains(value, "airborne"),
		strings.Contains(value, "arrived"), strings.Contains(value, "landed"):
		return domain.FlightStatusDeparted
	case strings.Contains(value, "board"):
		return domain.FlightStatusBoarding
	case strings.Contains(value, "delay"):
		return domain.FlightStatusDelayed
	case strings.Contains(value, "on_time"), strings.Contains(value, "ontime"), strings.Contains(value, "on-time"):
		return domain.FlightStatusOnTime
	case strings.Contains(value, "scheduled"), strings.Contains(value, "upcoming"), strings.Contains(value, "plan"):
		return domain.FlightStatusScheduled
	default:
		return domain.FlightStatusUnknown
	}
}

// fetchEOOB 打开 EOOB 的航班页，返回页面纯文本。
func (s *FlightStatusSkill) fetchEOOB(ctx context.Context, flight domain.Flight) (pageContent, error) {
	// 分两步，每步各用一个全新的浏览器会话。
	//
	// 为什么不用一个会话连着走：实测同一个会话里第二次导航会被 Cloudflare
	// 弹挑战页，而且重试也过不去 —— 一旦被标记就一直是挑战页。
	// 而每次会话的第一次导航都稳定。所以宁可多开一次浏览器。
	// 旅客只填航班号，航线由站点自己解析，我们不需要机场。
	number := strings.ToUpper(strings.TrimSpace(flight.Number))

	href, err := searchEOOBHref(ctx, number)
	if err != nil {
		return pageContent{}, err
	}

	return readEOOBPage(ctx, eoobBase+href)
}

// searchEOOBHref 用航班号搜索出航班级页面的地址。
//
// 步骤和等待时间是照搬手工验证过的流程，不要"优化"：
// 等页面就绪 13 秒 → 先清空再填 → 等 6 秒 → 取结果链接。
func searchEOOBHref(ctx context.Context, number string) (string, error) {
	number = strings.ToUpper(strings.TrimSpace(number))

	b, err := browser.Launch(ctx, browser.LaunchOptions{})
	if err != nil {
		return "", err
	}
	defer func() { _ = b.Close() }()

	if err := b.Navigate(eoobBase + "/hangban-zhuizong"); err != nil {
		return "", fmt.Errorf("打开 EOOB 航班追踪页失败: %w", err)
	}
	b.Sleep(13 * time.Second)

	if err := b.Fill("#liveflights-search-input", ""); err != nil {
		return "", err
	}
	if err := b.Fill("#liveflights-search-input", number); err != nil {
		return "", err
	}
	b.Sleep(6 * time.Second)

	href, err := waitEOOBFlightLink(b, number, 10*time.Second)
	if err != nil {
		return "", err
	}
	return href, nil
}

// readEOOBPage 用新会话直接打开航班级页面，按固定 id 读字段。
func readEOOBPage(ctx context.Context, url string) (pageContent, error) {
	b, err := browser.Launch(ctx, browser.LaunchOptions{})
	if err != nil {
		return pageContent{}, err
	}
	defer func() { _ = b.Close() }()

	if err := b.Navigate(url); err != nil {
		return pageContent{}, fmt.Errorf("打开 %s 失败: %w", url, err)
	}
	if err := b.WaitSelectorValue("#departure-scheduled-value", 40*time.Second); err != nil {
		return pageContent{}, err
	}

	var page pageContent
	page.text, _ = b.PageText()

	fields, err := readEOOBFields(b)
	if err != nil {
		return page, fmt.Errorf("读取航班字段失败: %w", err)
	}
	if fields.isZero() {
		return page, fmt.Errorf("页面上没有读到任何航班字段")
	}
	page.structured = &fields
	return page, nil
}

// waitEOOBFlightLink 在搜索结果里等航班链接出现。
//
// 链接形如 /CZ3101-CAN-PKX —— 航线是站点自己解析出来的，不是我们给的。
func waitEOOBFlightLink(b *browser.Browser, flightNumber string, timeout time.Duration) (string, error) {
	prefix := "/" + strings.ToUpper(strings.TrimSpace(flightNumber)) + "-"
	expr := fmt.Sprintf(`(() => {
		const links = [...document.querySelectorAll('a[href]')].map(a => a.getAttribute('href') || '')
		return links.find(h => h.toUpperCase().startsWith(%q)) || ''
	})()`, prefix)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		href, err := b.EvaluateString(expr)
		if err == nil {
			if href = strings.TrimSpace(href); href != "" {
				return href, nil
			}
		}
		time.Sleep(700 * time.Millisecond)
	}
	return "", fmt.Errorf("EOOB 搜索结果里没有出现航班 %s（可能该航班不在它的库里）", flightNumber)
}

// eoobReadScript 按固定 id 读字段。
//
// 为什么用 id 而不是让模型读文本：实测页面上同一个卡片里有两个时间
// （计划时间和看板更新时间，长得一模一样），模型换了两种提示词都会取错。
// 而页面本身给关键字段都带了稳定 id，直接取既准确又快，还省一次模型调用。
//
// 值为 "--" 表示该字段暂无数据，按空字符串处理。
const eoobReadScript = `(() => {
	const pick = (id) => {
		const el = document.getElementById(id)
		if (!el) return ''
		const t = (el.innerText || '').trim()
		if (t === '' || t === '--' || t === '\u2014' || t === '-') return ''
		return t
	}
	return JSON.stringify({
		headline:        pick('headline-row-1'),
		departureTime:   pick('departure-scheduled-value'),
		departureStatus: pick('departure-status-value'),
		departureGate:   pick('departure-gate-value'),
		arrivalGate:     pick('arrival-gate-value')
	})
})()`

// readEOOBFields 按固定 id 从页面上把航班字段读出来。
func readEOOBFields(b *browser.Browser) (structuredFields, error) {
	raw, err := b.EvaluateString(eoobReadScript)
	if err != nil {
		return structuredFields{}, err
	}

	var got struct {
		Headline        string `json:"headline"`
		DepartureTime   string `json:"departureTime"`
		DepartureStatus string `json:"departureStatus"`
		DepartureGate   string `json:"departureGate"`
		ArrivalGate     string `json:"arrivalGate"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return structuredFields{}, fmt.Errorf("解析页面字段失败: %w", err)
	}

	fields := structuredFields{
		DepartureTime: strings.TrimSpace(got.DepartureTime),
		Gate:          strings.TrimSpace(got.DepartureGate),
	}
	if fields.Gate == "" {
		fields.Gate = strings.TrimSpace(got.ArrivalGate)
	}

	// 状态优先取看板字段；它是 "--" 时退回标题行 "CZ3101 • 计划" 里那半句
	fields.StatusText = strings.TrimSpace(got.DepartureStatus)
	if fields.StatusText == "" {
		if i := strings.Index(got.Headline, "\u2022"); i >= 0 {
			fields.StatusText = strings.TrimSpace(got.Headline[i+3:])
		}
	}
	return fields, nil
}

// fetchAirline 走航司官网配方，返回结果页纯文本。
func (s *FlightStatusSkill) fetchAirline(ctx context.Context, flight domain.Flight) (pageContent, error) {
	recipe, ok := RecipeFor(flight.Number)
	if !ok {
		return pageContent{}, fmt.Errorf("%s 的官网配方尚未验证", FlightPrefix(flight.Number))
	}
	text, err := RunRecipe(ctx, recipe, flight.Number)
	return pageContent{text: text}, err
}

// flightExtract 是模型从页面文本里抽出来的字段。
//
// FlightStatusText 是页面上的状态原话（如「计划」「延误」「已于06:11起飞」），
// 不是契约枚举 —— 枚举由代码根据这句原话映射，避免模型自己「翻译」。
type flightExtract struct {
	FlightStatusText   string `json:"flightStatusText"`
	Gate               string `json:"gate"`
	ScheduledDeparture string `json:"scheduledDeparture"`
	BoardingTime       string `json:"boardingTime"`
	GateCloseTime      string `json:"gateCloseTime"`
}

// extract 从页面文本里抽字段，并逐项回原文校验。
func (s *FlightStatusSkill) extract(
	ctx context.Context,
	page pageContent,
	sourceName string,
	flight domain.Flight,
) (Result, error) {
	result := Result{}
	pageText := page.text

	// 页面上根本没出现这个航班号，说明这一页不是关于它的
	if !strings.Contains(strings.ToUpper(pageText), strings.ToUpper(flight.Number)) {
		result.Issues = append(result.Issues,
			fmt.Sprintf("%s 的页面上没有找到航班号 %s", sourceName, flight.Number))
		return result, nil
	}

	var extracted flightExtract
	if _, err := model.GenerateJSON(ctx, s.client, model.Request{
		Messages: []model.Message{
			{Role: "system", Content: flightExtractSystem},
			{Role: "user", Content: flightExtractPrompt(sourceName, pageText)},
		},
	}, &extracted); err != nil {
		return result, err
	}

	source := sourceName + "（" + flight.Number + "）"
	observedAt := time.Now().Format(time.RFC3339)

	// 状态：用页面原话映射，原话必须能在页面上找到
	if statusText := strings.TrimSpace(extracted.FlightStatusText); statusText != "" &&
		textutil.Contains(pageText, statusText) {
		if status := mapStatusText(statusText); status != domain.FlightStatusUnknown {
			result.Observations = append(result.Observations, Observation{
				Field: FieldFlightStatus, Value: status, Source: source,
				ObservedAt: observedAt, Confidence: "high",
			})
		}
	}

	// 登机口：短字符串，直接回原文校验
	if gate := verified(pageText, extracted.Gate); gate != "" {
		result.Observations = append(result.Observations, Observation{
			Field: FieldFlightGate, Value: gate, Source: source,
			ObservedAt: observedAt, Confidence: "high",
		})
	}

	// 时间：先校验原文，再补成 RFC3339。
	// 页面显示的是出发机场当地时间；时区来自 FlightIdentitySkill 打开的机场页。
	location := departureLocation(flight.From)
	times := FlightTimes{}
	// 计划起飞时间优先用页面结构直接读出来的值。
	// 只有当结构读取失败时，才退回去用模型抽的结果，并回原文校验。
	var departure string
	if page.structured != nil {
		departure = page.structured.DepartureTime
	}
	if departure == "" {
		departure = verified(pageText, extracted.ScheduledDeparture)
	}
	if departure != "" {
		times.DepartureTime = normalizeTimeInLocation(departure, flight.Date, location)
	}
	if v := verified(pageText, extracted.BoardingTime); v != "" {
		times.BoardingTime = normalizeTimeInLocation(v, flight.Date, location)
	}
	if v := verified(pageText, extracted.GateCloseTime); v != "" {
		times.GateCloseTime = normalizeTimeInLocation(v, flight.Date, location)
	}
	if times.DepartureTime != "" || times.BoardingTime != "" || times.GateCloseTime != "" {
		result.Observations = append(result.Observations, Observation{
			Field: FieldFlightTimes, Value: times, Source: source,
			ObservedAt: observedAt, Confidence: "high",
		})
	}

	if len(result.Observations) == 0 {
		result.Issues = append(result.Issues, sourceName+" 的页面上没有抽到可用字段")
	}
	return result, nil
}

// verified 回原文校验一个取值：找得到才返回，否则返回空串。
//
// 这一步是整套流程的信任边界：模型说的任何值，只要在页面原文里找不到，
// 一律当它不存在。
func verified(pageText, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !textutil.Contains(pageText, value) {
		return ""
	}
	return value
}

// mapStatusText 把页面上的状态原话映射到契约枚举。
//
// 注意这里映射的是「页面原话」而不是模型给的枚举 —— 让模型只负责抄原文，
// 翻译成枚举这种确定性的事交给代码，可复核。
func mapStatusText(raw string) string {
	text := strings.TrimSpace(raw)

	// 机场看板的写法常带时间，例如「已于06:11起飞」
	switch {
	case strings.Contains(text, "取消"):
		return domain.FlightStatusCancelled
	case strings.Contains(text, "备降"), strings.Contains(text, "返航"):
		return domain.FlightStatusDiverted
	case strings.Contains(text, "起飞"):
		return domain.FlightStatusDeparted
	case strings.Contains(text, "到达"), strings.Contains(text, "落地"):
		// 已经落地等于这班飞机走完了。契约里没有 arrived，
		// 对旅客而言「已经走了」和 departed 是同一个意思。
		return domain.FlightStatusDeparted
	case strings.Contains(text, "登机"):
		return domain.FlightStatusBoarding
	case strings.Contains(text, "延误"), strings.Contains(text, "晚点"):
		return domain.FlightStatusDelayed
	case strings.Contains(text, "计划"), strings.Contains(text, "预计"):
		return domain.FlightStatusScheduled
	case strings.Contains(text, "准点"), strings.Contains(text, "正常"), strings.Contains(text, "正点"):
		return domain.FlightStatusOnTime
	default:
		return domain.FlightStatusUnknown
	}
}

// normalizeTime 保留原有东八区行为，供单元测试和国内默认路径使用。
func normalizeTime(raw, date string) string {
	return normalizeTimeInLocation(raw, date, time.FixedZone("CST", 8*3600))
}

// normalizeTimeInLocation 把页面上的当地时间整理成契约要求的 RFC3339。
//
// 页面上通常是 "08:00" 或 "09月13日 08:00"，没有年份也没有时区。
// 行程日期我们知道；时区优先用出发机场页解析出的 location，拿不到才退回东八区。
// 拼不出来就返回空串。
func normalizeTimeInLocation(raw, date string, location *time.Location) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if _, err := time.Parse(time.RFC3339, raw); err == nil {
		return raw
	}
	if location == nil {
		location = time.FixedZone("CST", 8*3600)
	}

	day, err := time.Parse("2006-01-02", strings.TrimSpace(date))
	if err != nil {
		return ""
	}

	// "09月13日 08:00"
	if t, err := time.ParseInLocation("01月02日 15:04", raw, location); err == nil {
		return time.Date(day.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, location).Format(time.RFC3339)
	}
	// "09-13 08:00"
	if t, err := time.ParseInLocation("01-02 15:04", raw, location); err == nil {
		return time.Date(day.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, location).Format(time.RFC3339)
	}
	// 只有钟点："08:00"
	for _, layout := range []string{"15:04", "15:04:05"} {
		clock, err := time.Parse(layout, raw)
		if err != nil {
			continue
		}
		return time.Date(day.Year(), day.Month(), day.Day(),
			clock.Hour(), clock.Minute(), clock.Second(), 0, location).Format(time.RFC3339)
	}
	return ""
}

func departureLocation(iata string) *time.Location {
	if timezone, ok := cachedEOOBAirportTimezone(iata); ok {
		if location, err := time.LoadLocation(timezone); err == nil {
			return location
		}
	}
	// 时区解析不到时用 UTC，避免按错误的东八区生成国际航班时间。
	return time.UTC
}

const flightExtractSystem = `你是一个信息抽取器。用户会给你一段从航班信息网页抓取的文本，
你要把其中的航班字段原样抄出来。

规则：
1. 只抄文本里确实存在的值。不要推断、不要换算、不要补全、不要翻译。
2. 抄出来的值必须与文本中的写法完全一致。例如页面上写 "08:00"，
   你就写 "08:00"，不要改写成别的格式。
3. 文本里没有的字段，填空字符串。
4. flightStatusText 填页面上表示航班状态的原话，例如 "计划"、"延误"、
   "已于06:11起飞"。
5. 页面同时有「出发看板」和「到达看板」时，scheduledDeparture 只取出发的那张。
   取「计划」这个词后面紧跟着的时间。
6. 看板卡片末尾往往还有一个时间（那是看板的更新时间，不是航班时间），不要取它。
7. 登机口如果写的是"没有可用的登机口记录"之类的话，填空字符串。
8. 拿不准的字段宁可填空字符串，也不要猜。

只输出 JSON，不要解释文字，不要 markdown 代码块。`

func flightExtractPrompt(sourceName, pageText string) string {
	return fmt.Sprintf(`下面是从%s抓取的航班页面文本，请抽取字段。

输出 JSON：
{
  "flightStatusText": "页面上的状态原话",
  "gate": "登机口",
  "scheduledDeparture": "计划起飞时间",
  "boardingTime": "开始登机时间",
  "gateCloseTime": "登机口关闭时间"
}

页面文本：
---
%s
---`, sourceName, pageText)
}
