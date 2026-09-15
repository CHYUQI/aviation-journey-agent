// Command modelcheck 是最小连通性自检：发一句话给配置好的模型，打印回复与耗时。
//
// 用法：
//
//	cd backend
//	go run ./cmd/modelcheck "你好"                     # 基础连通
//	go run ./cmd/modelcheck -json "输出一个 JSON"        # 验证 JSON 模式
//
// 用来在接入 Agent 之前确认端点、key、模型名和各项能力都可用。
// 服务本身不依赖模型可用性，这里只是把问题提前暴露出来。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"aviation-journey-agent/backend/internal/config"
	"aviation-journey-agent/backend/internal/model"
)

func main() {
	jsonMode := flag.Bool("json", false, "要求模型输出 JSON（response_format=json_object）")
	flag.Parse()

	cfg := config.Load()
	if !cfg.Model.Configured() {
		log.Fatal("模型未配置：请把 backend/.env.example 复制为 backend/.env，并填写 MODEL_BASE_URL / MODEL_API_KEY / MODEL_NAME")
	}

	prompt := "你好，请用一句话说明你能做什么"
	if args := flag.Args(); len(args) > 0 {
		prompt = strings.Join(args, " ")
	}

	client := model.NewOpenAI(model.Options{
		BaseURL:     cfg.Model.BaseURL,
		APIKey:      cfg.Model.APIKey,
		Model:       cfg.Model.Name,
		Timeout:     cfg.Model.Timeout,
		Temperature: cfg.Model.Temperature,
	})

	req := model.Request{
		Messages: []model.Message{{Role: "user", Content: prompt}},
		JSONMode: *jsonMode,
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Model.Timeout)
	defer cancel()

	log.Printf("模型：%s @ %s", cfg.Model.Name, cfg.Model.BaseURL)
	if *jsonMode {
		log.Println("模式：JSON")
	}

	start := time.Now()
	resp, err := client.Chat(ctx, req)
	if err != nil {
		log.Fatalf("调用失败：%v", err)
	}
	elapsed := time.Since(start).Round(time.Millisecond)

	fmt.Printf("\n耗时：%s\n", elapsed)
	fmt.Printf("原始输出：\n%s\n", resp.Content)

	// JSON 模式下顺手校验一次，确认拿到的确实是合法 JSON
	if *jsonMode {
		var probe any
		if err := json.Unmarshal([]byte(resp.Content), &probe); err != nil {
			fmt.Printf("\n[失败] 输出不是合法 JSON：%v\n", err)
			os.Exit(1)
		}
		fmt.Println("\n[通过] 输出是合法 JSON")
	}
}
