// Command browsercheck 用来调试航司配方。
//
// 两种用法：
//
//	go run ./cmd/browsercheck CZ3101                  # 按配方查一次航班，打印结果页
//	go run ./cmd/browsercheck -url <网址>             # 打开任意网页，列出可交互元素
//
// 调通一家新航司的流程：
//  1. 用 -url 打开候选的查询页，看页面上有什么输入框和按钮；
//  2. 写一份配方（见 internal/skill/airline.go）；
//  3. 用第一种种用法验证，直到结果页能正确打印出来。
//
// 它只做浏览器抓取，不调用模型，方便把"抓不到"和"抽不对"分开定位。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"aviation-journey-agent/backend/internal/browser"
	"aviation-journey-agent/backend/internal/skill"
)

func main() {
	url := flag.String("url", "", "打开这个网址并列出可交互元素（探索模式）")
	wait := flag.Duration("wait", 8*time.Second, "探索模式下等页面渲染的时间")
	flag.Parse()

	if *url != "" {
		explore(*url, *wait)
		return
	}

	flightNo := "CZ3101"
	if args := flag.Args(); len(args) > 0 {
		flightNo = strings.ToUpper(strings.TrimSpace(args[0]))
	}

	recipe, ok := skill.RecipeFor(flightNo)
	if !ok {
		log.Fatalf("航班号 %s 的官网配方尚未验证。已实测通过：%s",
			flightNo, strings.Join(skill.VerifiedAirlines(), "、"))
	}

	log.Printf("航司：%s  航班号：%s（查询框填 %s）", recipe.Airline, flightNo, skill.FlightNumberDigits(flightNo))

	text, err := skill.RunRecipe(context.Background(), recipe, flightNo)
	if err != nil {
		log.Fatalf("查询失败：%v", err)
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Println(text)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("页面文本 %d 字符\n", len(text))
}

// explore 打开一个网址，列出页面上的输入框和按钮。
func explore(url string, wait time.Duration) {
	b, err := browser.Launch(context.Background(), browser.LaunchOptions{})
	if err != nil {
		log.Fatalf("启动浏览器失败：%v", err)
	}
	defer func() { _ = b.Close() }()

	if err := b.Navigate(url); err != nil {
		log.Fatalf("打开页面失败：%v", err)
	}
	b.Sleep(wait)

	title, _ := b.EvaluateString("document.title")
	fmt.Printf("网址：%s\n标题：%s\n\n", url, title)

	elements, _ := b.EvaluateString(`
		[...document.querySelectorAll('input,select,button,a')].slice(0, 30).map((el, i) =>
			i + '. <' + el.tagName.toLowerCase() + '> type=' + (el.type || '') +
			' placeholder=' + JSON.stringify(el.placeholder || '') +
			' text=' + JSON.stringify((el.innerText || '').trim().slice(0, 24))
		).join('\n')
	`)
	fmt.Println("可交互元素：")
	fmt.Println(elements)

	if text, err := b.PageText(); err == nil {
		runes := []rune(strings.TrimSpace(text))
		if len(runes) > 400 {
			runes = runes[:400]
		}
		fmt.Printf("\n页面文本前 400 字：\n%s\n", string(runes))
	}

	_ = os.Stdout.Sync()
}
