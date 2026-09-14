package skill

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/model"
	"aviation-journey-agent/backend/internal/textutil"
)

// FlightTimes 是航班的关键时间节点，用于生成 State.Timeline。
type FlightTimes struct {
	DepartureTime string // RFC3339
	BoardingTime  string // RFC3339
	GateCloseTime string // RFC3339
}

// FlightStatusSkill 查询航班动态：状态、登机口、计划起飞时间。
//
// 数据来源是航司官网，不是搜索引擎。原因：实测搜索引擎给的航班时刻会过时
// （查到 CZ6656 "每日执飞 13:05"，而南航官网说当天没有这个航班），
// 官网才是第一手数据。
//
// 三步走：
//  1. 渲染与交互：用本机 Edge 无头模式打开官网查询页，填航班号、点查询。
//     航司页面是 JS 渲染的，普通 HTTP 抓取只能拿到空壳。
//  2. 抽取：把结果页的可见文本交给模型，让它按固定结构抽字段。
//     不为每家航司写解析器 —— DOM 结构各不相同还会改版。
//  3. 校验：每个抽出来的值都要回到页面原文里找得到，找不到就丢弃。
//     模型负责适应页面，代码负责保证它没瞎编。
type FlightStatusSkill struct {
	client model.Client
}

func NewFlightStatusSkill(client model.Client) *FlightStatusSkill {
	return &FlightStatusSkill{client: client}
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

// flightExtract 是模型从官网页面文本里抽出来的字段。
type flightExtract struct {
	// FlightStatusText 是页面上的状态原话（如"到达""延误"），
	// 不是枚举值 —— 枚举由代码根据这句原话映射，避免模型自己"翻译"。
	FlightStatusText   string `json:"flightStatusText"`
	Gate               string `json:"gate"`
	ScheduledDeparture string `json:"scheduledDeparture"`
	ActualDeparture    string `json:"actualDeparture"`
	BoardingTime       string `json:"boardingTime"`
	GateCloseTime      string `json:"gateCloseTime"`
	SourceQuote        string `json:"sourceQuote"`
}

func (s *FlightStatusSkill) Execute(ctx context.Context, query Query) (Result, error) {
	result := Result{}

	if len(query.Journey.Flights) == 0 {
		result.Issues = append(result.Issues, "行程里没有航段，无法查询航班动态")
		return result, nil
	}
	flight := query.Journey.Flights[0]

	recipe, ok := RecipeFor(flight.Number)
	if !ok {
		// 说法要准确：这是"还没验证"而不是"不支持"。
		// 查询引擎本身是通用的，只是这家的页面配方还没调过。
		result.Issues = append(result.Issues, fmt.Sprintf(
			"%s 官网的查询配方尚未验证（已实测通过：%s），本次无法自动获取航班动态",
			FlightPrefix(flight.Number), strings.Join(VerifiedAirlines(), "、")))
		return result, nil
	}

	pageText, err := RunRecipe(ctx, recipe, flight.Number)
	if err != nil {
		result.Issues = append(result.Issues, "查询航班动态失败："+err.Error())
		return result, nil
	}

	// 官网明确说没这个航班：如实上报，不要退回去猜
	if strings.Contains(pageText, "没有符合条件") {
		result.Issues = append(result.Issues, fmt.Sprintf(
			"%s官网没有查到 %s，请核对航班号与日期", recipe.Airline, flight.Number))
		return result, nil
	}

	if s.client == nil {
		result.Issues = append(result.Issues, "未配置模型，无法解析官网页面内容")
		return result, nil
	}

	var extracted flightExtract
	if _, err := model.GenerateJSON(ctx, s.client, model.Request{
		Messages: []model.Message{
			{Role: "system", Content: flightExtractSystem},
			{Role: "user", Content: flightExtractPrompt(recipe.Airline, pageText)},
		},
	}, &extracted); err != nil {
		result.Issues = append(result.Issues, "解析官网页面失败："+err.Error())
		return result, nil
	}

	source := recipe.Airline + "官网"
	observedAt := time.Now().Format(time.RFC3339)

	// 状态：模型给的是页面原话，用它做映射；原话必须能在页面上找到
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
	if gate := strings.TrimSpace(extracted.Gate); gate != "" && textutil.Contains(pageText, gate) {
		result.Observations = append(result.Observations, Observation{
			Field: FieldFlightGate, Value: gate, Source: source,
			ObservedAt: observedAt, Confidence: "high",
		})
	}

	// 时间：先校验原文，再补成 RFC3339
	times := FlightTimes{}
	if v := verified(pageText, extracted.ScheduledDeparture); v != "" {
		times.DepartureTime = normalizeTime(v, flight.Date)
	}
	if v := verified(pageText, extracted.BoardingTime); v != "" {
		times.BoardingTime = normalizeTime(v, flight.Date)
	}
	if v := verified(pageText, extracted.GateCloseTime); v != "" {
		times.GateCloseTime = normalizeTime(v, flight.Date)
	}
	if times.DepartureTime != "" || times.BoardingTime != "" || times.GateCloseTime != "" {
		result.Observations = append(result.Observations, Observation{
			Field: FieldFlightTimes, Value: times, Source: source,
			ObservedAt: observedAt, Confidence: "high",
		})
	}

	if len(result.Observations) == 0 {
		result.Issues = append(result.Issues, "官网页面上没有抽到可用的字段")
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

// mapStatusText 把官网上的状态原话映射到契约枚举。
//
// 注意这里映射的是"页面原话"而不是模型给的枚举 —— 让模型只负责抄原文，
// 翻译成枚举这种确定性的事交给代码，可复核。
func mapStatusText(raw string) string {
	switch strings.TrimSpace(raw) {
	case "计划", "计划中", "预计":
		return domain.FlightStatusScheduled
	case "正常", "准点", "正点":
		return domain.FlightStatusOnTime
	case "延误", "晚点", "延误起飞":
		return domain.FlightStatusDelayed
	case "登机", "登机中", "正在登机":
		return domain.FlightStatusBoarding
	case "起飞", "已起飞", "离地":
		return domain.FlightStatusDeparted
	case "到达", "已到达", "落地":
		// 已经落地等于这班飞机走完了。契约里没有 arrived，
		// 对旅客而言"已经走了"和 departed 是同一个意思。
		return domain.FlightStatusDeparted
	case "取消", "已取消", "航班取消":
		return domain.FlightStatusCancelled
	case "备降", "返航":
		return domain.FlightStatusDiverted
	default:
		return domain.FlightStatusUnknown
	}
}

// normalizeTime 把官网上的时间整理成契约要求的 RFC3339。
//
// 官网上通常是 "09月13日 08:00" 或 "08:00"，没有年份也没有时区。
// 行程日期我们是知道的，国内航班统一按东八区处理；拼不出来就返回空串。
func normalizeTime(raw, date string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if _, err := time.Parse(time.RFC3339, raw); err == nil {
		return raw
	}

	day, err := time.Parse("2006-01-02", strings.TrimSpace(date))
	if err != nil {
		return ""
	}

	// 官网常见写法："09月13日 08:00"
	if t, err := time.ParseInLocation("01月02日 15:04", raw, time.FixedZone("CST", 8*3600)); err == nil {
		return time.Date(day.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0,
			time.FixedZone("CST", 8*3600)).Format(time.RFC3339)
	}

	// 只有钟点："08:00"
	for _, layout := range []string{"15:04", "15:04:05"} {
		clock, err := time.Parse(layout, raw)
		if err != nil {
			continue
		}
		return time.Date(day.Year(), day.Month(), day.Day(),
			clock.Hour(), clock.Minute(), clock.Second(), 0,
			time.FixedZone("CST", 8*3600)).Format(time.RFC3339)
	}

	return ""
}

const flightExtractSystem = `你是一个信息抽取器。用户会给你一段从航司官网抓取的航班动态页面文本，
你要把其中的字段原样抽取出来。

规则：
1. 只抽取文本里确实存在的值。不要推断、不要换算、不要补全、不要翻译。
2. 抽出来的值必须与文本中的写法一致，例如页面上写 "09月13日 08:00"，
   你就写 "09月13日 08:00"，不要改写成别的格式。
3. 文本里没有的字段，填空字符串。
4. flightStatusText 填页面上表示航班状态的那个词本身，例如 "到达"、"延误"、"计划"。

只输出 JSON，不要解释文字，不要 markdown 代码块。`

func flightExtractPrompt(airline, pageText string) string {
	return fmt.Sprintf(`下面是从%s官网抓取的航班动态页面文本，请抽取字段。

输出 JSON：
{
  "flightStatusText": "页面上的状态原话",
  "gate": "登机口",
  "scheduledDeparture": "计划起飞时间",
  "actualDeparture": "实际起飞时间",
  "boardingTime": "开始登机时间",
  "gateCloseTime": "登机口关闭时间",
  "sourceQuote": "你依据的原文片段"
}

页面文本：
---
%s
---`, airline, pageText)
}
