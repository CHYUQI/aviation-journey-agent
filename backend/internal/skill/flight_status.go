package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aviation-journey-agent/backend/internal/domain"
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
	ArrivalTime   string // RFC3339
	BoardingTime  string // RFC3339
	GateCloseTime string // RFC3339
}

// FlightStatusSkill 查询航班动态：状态、登机口、计划起飞时间。
//
// 只走 EOOB 的状态 JSON 接口（apil.eoob.com）：
//   - 一个站覆盖所有航司，直接返回结构化字段，不需要模型抽取；
//   - 不打开任何网页。eoob.com.cn 的 Cloudflare 对无头浏览器一律下发 managed
//     challenge（实测干等 40s 也等不到元素），航司官网配方走的也是无头浏览器，
//     南航「按航班号」点击失败、东航等待超时，这条路已经没有可用性。
//
// 所以旅客链路里不再有"航司官网"这一环：接口查不到就如实报 issue，由上层降级。
// 配方表与 cmd/browsercheck 仍然保留，只作为"某家航司官网还能不能自动查"的
// 验证工具，不参与行程分析。
type FlightStatusSkill struct {
	HTTPClient *http.Client
}

func NewFlightStatusSkill() *FlightStatusSkill {
	return &FlightStatusSkill{HTTPClient: &http.Client{Timeout: 15 * time.Second}}
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

	// 接口没返回可用字段：如实说明，不再回退航司官网（浏览器路径已整体移除）。
	// 常见原因：该日期没有这个航班、航线写错，或 EOOB 还没收录这条动态。
	return Result{Issues: []string{
		fmt.Sprintf("EOOB 状态接口没有返回 %s（%s %s→%s）的可用动态",
			strings.ToUpper(strings.TrimSpace(flight.Number)), flight.Date, flight.From, flight.To),
	}}, nil
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
	setEOOBHeaders(req, "application/json", eoobJSONRequest)
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

	// 到达时间：接口给的是绝对时刻，按到达机场时区展示（拿不到时区就沿用出发地）。
	if payload.Schedule.ArrivalUTC > 0 {
		arrivalLocation := location
		if timezone, timezoneErr := resolveEOOBAirportTimezone(ctx, to); timezoneErr == nil {
			if resolved, loadErr := time.LoadLocation(timezone); loadErr == nil {
				arrivalLocation = resolved
			}
		}
		arrival := time.Unix(payload.Schedule.ArrivalUTC, 0).In(arrivalLocation)
		if arrival.After(time.Unix(payload.Schedule.DepartureUTC, 0)) || payload.Schedule.DepartureUTC == 0 {
			times.ArrivalTime = arrival.Format(time.RFC3339)
		}
	}

	// 登机/关闸时间接口不提供，按行业惯例从计划起飞时间推算，并写进 issues。
	// 契约要求：保守假设必须让旅客看得见，不能当成航司公布的时间。
	if boarding, gateClose, ok := deriveBoardingTimes(times.DepartureTime); ok {
		times.BoardingTime = boarding
		times.GateCloseTime = gateClose
		timezoneIssues = append(timezoneIssues,
			"开始登机/登机口关闭时间按计划起飞前 40/15 分钟推算（行业惯例），非航司公布时间")
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

// normalizeTimeInLocation 把页面/接口里的时间写法补成带时区的 RFC3339。
// 时区来自出发机场（EOOB 机场页），拿不到就让调用方传 UTC。
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

// deriveBoardingTimes 从计划起飞时间推算开始登机与登机口关闭时间。
//
// 口径：国内航班常见做法 —— 起飞前 40 分钟开始登机、起飞前 15 分钟关闭登机口，
// 与契约示例（14:20 / 14:45 / 15:00）一致。返回值 ok=false 表示起飞时间不可解析。
func deriveBoardingTimes(departureRFC3339 string) (string, string, bool) {
	departure, err := time.Parse(time.RFC3339, strings.TrimSpace(departureRFC3339))
	if err != nil {
		return "", "", false
	}
	return departure.Add(-40 * time.Minute).Format(time.RFC3339),
		departure.Add(-15 * time.Minute).Format(time.RFC3339), true
}
