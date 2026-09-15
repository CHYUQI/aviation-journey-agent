package main

import (
	"context"
	"log"
	"time"

	"github.com/gin-gonic/gin"

	adviceagent "aviation-journey-agent/backend/internal/agent/advice"
	dataagent "aviation-journey-agent/backend/internal/agent/data"
	"aviation-journey-agent/backend/internal/api"
	"aviation-journey-agent/backend/internal/config"
	"aviation-journey-agent/backend/internal/model"
	"aviation-journey-agent/backend/internal/runtime"
	"aviation-journey-agent/backend/internal/skill"
	"aviation-journey-agent/backend/internal/store"
)

// refreshInterval 是定期刷新周期。
// 位置或阶段变化后的推送不依赖它，走 SSEHub 立即推送。
const refreshInterval = 15 * time.Second

func main() {
	cfg := config.Load()
	// 模型调用并发上限：默认串行，避免并发打爆端点导致 45s+ 超时（见 internal/model/limiter.go）
	model.SetMaxConcurrency(cfg.Model.MaxConcurrency)

	memoryStore := store.NewMemoryStore()

	skillRegistry := skill.NewRegistry()
	skillRegistry.Register(skill.NewFlightStatusSkill())
	skillRegistry.Register(skill.NewFlightIdentitySkill())
	skillRegistry.Register(skill.NewAirportStatusSkill())
	skillRegistry.Register(skill.RouteETASkill{})

	dataAgent := dataagent.NewAgent(skillRegistry)
	adviceAgent := adviceagent.NewAgent(newModelClient(cfg))

	hub := runtime.NewSSEHub()
	journeyRuntime := runtime.New(memoryStore, dataAgent, adviceAgent, hub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	journeyRuntime.StartPeriodicRefresh(ctx, refreshInterval, memoryStore.ListJourneyIDs)

	router := gin.Default()
	api.RegisterRoutes(router, journeyRuntime)

	log.Printf("aviation-journey-agent backend listening on %s", cfg.Addr)
	if err := router.Run(cfg.Addr); err != nil {
		log.Fatal(err)
	}
}

// newModelClient 按配置构造模型客户端。
//
// 没配 key 时返回 nil，Agent 会走降级路径（risk = unknown + 说明原因），
// 服务照常启动 —— 这样没有 key 的同学也能跑通整条链路。
func newModelClient(cfg config.Config) model.Client {
	if !cfg.Model.Configured() {
		log.Println("提示：模型未配置。复制 backend/.env.example 为 backend/.env 并填写 MODEL_API_KEY 即可启用。")
		return nil
	}

	log.Printf("模型：%s @ %s（超时 %s，温度 %.1f）",
		cfg.Model.Name, cfg.Model.BaseURL, cfg.Model.Timeout, cfg.Model.Temperature)

	return model.NewOpenAI(model.Options{
		BaseURL:     cfg.Model.BaseURL,
		APIKey:      cfg.Model.APIKey,
		Model:       cfg.Model.Name,
		Timeout:     cfg.Model.Timeout,
		Temperature: cfg.Model.Temperature,
	})
}
