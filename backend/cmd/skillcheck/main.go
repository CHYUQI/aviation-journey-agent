// Command skillcheck 直接跑一次航班技能，打印原始观测值与问题。
//
// 用途：把"抓不到"和"抽不对"两类问题分开定位。
// browsercheck 只看浏览器能不能拿到页面，这个看整条技能链路。
//
//	go run ./cmd/skillcheck CZ3101 2026-09-13
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"aviation-journey-agent/backend/internal/config"
	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/model"
	"aviation-journey-agent/backend/internal/skill"
)

func main() {
	flightNo := "CZ3101"
	if len(os.Args) > 1 {
		flightNo = strings.ToUpper(strings.TrimSpace(os.Args[1]))
	}
	date := time.Now().Format("2006-01-02")
	if len(os.Args) > 2 {
		date = os.Args[2]
	}

	cfg := config.Load()
	if !cfg.Search.Configured() {
		log.Fatal("联网检索未配置，请检查 backend/.env")
	}

	client := model.NewDashScope(model.DashScopeOptions{
		BaseURL:     cfg.Search.BaseURL,
		APIKey:      cfg.Search.APIKey,
		Model:       cfg.Search.Model,
		Timeout:     cfg.Search.Timeout,
		Temperature: cfg.Model.Temperature,
	})

	journey := domain.Journey{
		ID:         "debug",
		Flights:    []domain.Flight{{Number: flightNo, Date: date, From: "CAN", To: "PKX"}},
		HasBaggage: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	start := time.Now()
	result, err := skill.NewFlightStatusSkill(client).Execute(ctx, skill.Query{Journey: journey})
	fmt.Printf("耗时 %s\n\n", time.Since(start).Round(time.Second))

	if err != nil {
		fmt.Printf("执行出错: %v\n", err)
	}
	fmt.Printf("观测值 %d 条:\n", len(result.Observations))
	for _, o := range result.Observations {
		raw, _ := json.Marshal(o.Value)
		fmt.Printf("  field=%s  value=%s  source=%s  confidence=%s\n", o.Field, raw, o.Source, o.Confidence)
	}
	fmt.Printf("\n问题 %d 条:\n", len(result.Issues))
	for _, i := range result.Issues {
		fmt.Printf("  - %s\n", i)
	}
}
