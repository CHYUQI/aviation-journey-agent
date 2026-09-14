package skill

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aviation-journey-agent/backend/internal/browser"
)

// AirlineRecipe 是一家航司的航班动态"查询配方"。
//
// 设计要点：配方是数据，不是代码。加一家航司 = 加一份配方，
// 执行器不用改。航司页面改版时，也只改配方里的文字和选择器。
type AirlineRecipe struct {
	Airline string
	// Prefixes 是航司二字码，用来把航班号路由到官网
	Prefixes []string
	EntryURL string
	// MountWait 是等 SPA 挂载的时间。页面没挂载完就点会点空
	MountWait time.Duration
	Steps     []RecipeStep
}

// RecipeStep 是配方里的一步操作。
type RecipeStep struct {
	// Action 取值：click（按文字点击）、clickSelector（按选择器点击）、
	// fill（按选择器填值）、wait（等页面出现某段文字）、waitSelector（等元素出现）
	Action string
	// Target：click 用要点击的文字，fill 用 CSS 选择器，wait 用正则
	Target string
	// Value 仅 fill 使用，支持 {flightNo} 占位（已去掉航司前缀的数字部分）
	Value   string
	Timeout time.Duration
}

// airlineRecipes 是「已实测通过」的航司配方表。
//
// 注意：这张表表示验证进度，不是能力边界。
// 查询引擎对所有航司通用，只是每家的页面要单独调一遍配方。
// 没在这张表里的航司，含义是「还没测过」，不是「不支持」——
// 不能因为没测过就对外宣称不支持，那样会把未验证说成已知结论。
//
// 加一家航司：用 cmd/browsercheck 把配方调通，再补到这里。
var airlineRecipes = []AirlineRecipe{
	{
		Airline:   "中国东方航空",
		Prefixes:  []string{"MU"},
		EntryURL:  "https://m.ceair.com/flight/index",
		MountWait: 8 * time.Second,
		Steps: []RecipeStep{
			{Action: "waitSelector", Target: "input[type=text]", Timeout: 15 * time.Second},
			{Action: "fill", Target: "input[type=text]", Value: "{flightNo}"},
			{Action: "click", Target: "查询"},
			{Action: "sleep", Target: "10s"},
		},
	},
	{
		Airline:   "中国国际航空",
		Prefixes:  []string{"CA"},
		EntryURL:  "https://m.airchina.com.cn/ac/c/invoke/qryFlightDyns@pg",
		MountWait: 9 * time.Second,
		Steps: []RecipeStep{
			{Action: "waitSelector", Target: "input[placeholder='请输入航班号数字']", Timeout: 15 * time.Second},
			{Action: "fill", Target: "input[placeholder='请输入航班号数字']", Value: "{flightNo}"},
			// 提交按钮是 <input type="button"> 且没有文字，只能按选择器点
			{Action: "clickSelector", Target: "input[type=button]", Timeout: 10 * time.Second},
			{Action: "sleep", Target: "10s"},
		},
	},
	{
		Airline:   "中国南方航空",
		Prefixes:  []string{"CZ"},
		EntryURL:  "https://m.csair.com/flightstatus_new/#/",
		MountWait: 7 * time.Second,
		Steps: []RecipeStep{
			{Action: "click", Target: "按航班号"},
			{Action: "waitSelector", Target: "input[type=text]", Timeout: 15 * time.Second},
			{Action: "fill", Target: "input[type=text]", Value: "{flightNo}"},
			{Action: "click", Target: "开始查询"},
			{Action: "wait", Target: "计划起飞|没有符合条件", Timeout: 30 * time.Second},
		},
	},
}

// RecipeFor 按航班号的航司二字码找配方。
func RecipeFor(flightNumber string) (AirlineRecipe, bool) {
	prefix := FlightPrefix(flightNumber)
	if prefix == "" {
		return AirlineRecipe{}, false
	}
	for _, recipe := range airlineRecipes {
		for _, p := range recipe.Prefixes {
			if p == prefix {
				return recipe, true
			}
		}
	}
	return AirlineRecipe{}, false
}

// VerifiedAirlines 返回已实测通过的航司名。
//
// 给旅客解释"为什么这次查不到"时说「已实测通过的航司」而不是「支持」，
// 因为我们并不知道未验证的航司行不行。
func VerifiedAirlines() []string {
	names := make([]string, 0, len(airlineRecipes))
	for _, r := range airlineRecipes {
		names = append(names, r.Airline)
	}
	return names
}

// FlightPrefix 取航班号的航司二字码。
func FlightPrefix(flightNumber string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(flightNumber))
	if len(trimmed) < 3 {
		return ""
	}
	prefix := trimmed[:2]
	for _, r := range prefix {
		if r < 'A' || r > 'Z' {
			return ""
		}
	}
	return prefix
}

// FlightNumberDigits 去掉航司前缀，只留数字部分。
// 航司查询框通常要 "3101" 而不是 "CZ3101"。
func FlightNumberDigits(flightNumber string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(flightNumber)) {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// RunRecipe 按配方驱动浏览器查一次，返回结果页的可见文本。
//
// 返回文本而不是结构化数据，是因为每家航司的 DOM 结构都不一样、
// 还会频繁改版。让模型从文本里抽字段、代码再回原文校验，
// 比为每家航司写解析器更耐用。
func RunRecipe(ctx context.Context, recipe AirlineRecipe, flightNumber string) (string, error) {
	b, err := browser.Launch(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = b.Close() }()

	if err := b.Navigate(recipe.EntryURL); err != nil {
		return "", fmt.Errorf("打开%s官网失败: %w", recipe.Airline, err)
	}
	b.Sleep(recipe.MountWait)

	digits := FlightNumberDigits(flightNumber)
	for i, step := range recipe.Steps {
		switch step.Action {
		case "click":
			if err := b.ClickText(step.Target); err != nil {
				return "", fmt.Errorf("第 %d 步点击失败: %w", i+1, err)
			}
		case "clickSelector":
			if err := b.ClickSelector(step.Target); err != nil {
				return "", fmt.Errorf("第 %d 步点击失败: %w", i+1, err)
			}
		case "sleep":
			// 等待结果渲染。SPA 的"加载中"状态没有稳定特征可等，
			// 硬等一段时间是最省事也最不容易误判的做法。
			d, err := time.ParseDuration(step.Target)
			if err != nil {
				return "", fmt.Errorf("第 %d 步 sleep 参数不是合法时长: %q", i+1, step.Target)
			}
			b.Sleep(d)
		case "fill":
			value := strings.ReplaceAll(step.Value, "{flightNo}", digits)
			if err := b.Fill(step.Target, value); err != nil {
				return "", fmt.Errorf("第 %d 步填写失败: %w", i+1, err)
			}
		case "waitSelector":
			timeout := step.Timeout
			if timeout <= 0 {
				timeout = 20 * time.Second
			}
			if err := b.WaitSelector(step.Target, timeout); err != nil {
				return "", fmt.Errorf("第 %d 步等待元素失败: %w", i+1, err)
			}
		case "wait":
			timeout := step.Timeout
			if timeout <= 0 {
				timeout = 20 * time.Second
			}
			if err := b.WaitText(step.Target, timeout); err != nil {
				return "", fmt.Errorf("第 %d 步等待失败: %w", i+1, err)
			}
		default:
			return "", fmt.Errorf("第 %d 步动作未知: %q", i+1, step.Action)
		}
	}

	return b.PageText()
}
