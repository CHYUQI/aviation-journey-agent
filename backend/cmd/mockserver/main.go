// Command mockserver 是前端联调用的假服务端。
//
// 它不调用大模型、不访问外部数据源、不包含任何真实航班数据，
// 只按 docs/api/openapi.yaml 的结构返回固定夹具，用来验证前端在
// processing / ready / failed 三态下的渲染、SSE 推送，以及手动确认阶段的交互。
//
// 结构与真服务共用 internal/domain，保证两边不可能对不上。
//
//	go run ./cmd/mockserver      # 监听 :8081
//
// 前端联调：MOCK_API=1 npm run dev
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"aviation-journey-agent/backend/internal/domain"
)

const (
	addr         = ":8081"
	analyzeDelay = 2 * time.Second  // 模拟两个 Agent 的分析耗时
	pollEvery    = 1 * time.Second  // mock 内部检查快照是否变化
	heartbeat    = 15 * time.Second // 无变化时的心跳间隔
)

// record 是 mock 的内部状态：当前快照 + 契约之外的位置/阶段信息。
type record struct {
	snapshot domain.JourneySnapshot
	progress domain.JourneyProgress
}

var (
	mu      sync.RWMutex
	records = map[string]*record{}
)

func put(id string, rec *record) {
	mu.Lock()
	defer mu.Unlock()
	records[id] = rec
}

func load(id string) (*record, bool) {
	mu.RLock()
	defer mu.RUnlock()
	rec, ok := records[id]
	return rec, ok
}

func now() string       { return time.Now().Format(time.RFC3339) }
func ptr[T any](v T) *T { return &v }

// ---------- handler ----------

func create(c *gin.Context) {
	var req domain.CreateJourneyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请求体格式错误")
		return
	}
	if err := req.Validate(); err != nil {
		badRequest(c, err.Error())
		return
	}

	journey := domain.Journey{
		ID:         "j_" + strings.ToLower(req.Flights[0].Number),
		Flights:    req.Flights,
		HasBaggage: req.HasBaggage,
	}

	put(journey.ID, &record{snapshot: processingSnapshot(journey)})
	go finish(journey, domain.JourneyProgress{})

	c.JSON(http.StatusAccepted, gin.H{"journeyId": journey.ID, "status": domain.StatusProcessing})
}

// updateLocation 同时支持上报定位坐标与手动确认阶段。
func updateLocation(c *gin.Context) {
	id := c.Param("id")
	rec, ok := load(id)
	if !ok {
		notFound(c)
		return
	}

	var req domain.UpdateLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请求体格式错误")
		return
	}
	if err := req.Validate(); err != nil {
		badRequest(c, err.Error())
		return
	}

	progress := rec.progress
	if req.HasCoordinate() {
		progress.Location = &domain.Coordinate{Lat: *req.Lat, Lng: *req.Lng}
		progress.ManualStage = "" // 上报坐标后以定位为准
	}
	if req.Stage != nil {
		progress.ManualStage = *req.Stage
	}

	journey := rec.snapshot.Journey
	put(id, &record{snapshot: processingSnapshot(journey), progress: progress})
	go finish(journey, progress)

	c.JSON(http.StatusAccepted, gin.H{"journeyId": id, "status": domain.StatusProcessing})
}

func getState(c *gin.Context) {
	rec, ok := load(c.Param("id"))
	if !ok {
		notFound(c)
		return
	}
	c.JSON(http.StatusOK, rec.snapshot)
}

func stream(c *gin.Context) {
	id := c.Param("id")
	rec, ok := load(id)
	if !ok {
		notFound(c)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	fmt.Fprint(c.Writer, "retry: 5000\n\n")
	c.Writer.Flush()

	sendSnapshot(c, rec.snapshot)
	last := rec.snapshot.State.UpdatedAt

	poll := time.NewTicker(pollEvery)
	hb := time.NewTicker(heartbeat)
	defer poll.Stop()
	defer hb.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-poll.C:
			next, ok := load(id)
			if !ok {
				return
			}
			if next.snapshot.State.UpdatedAt != last {
				last = next.snapshot.State.UpdatedAt
				sendSnapshot(c, next.snapshot)
			}
		case <-hb.C:
			payload, _ := json.Marshal(gin.H{"at": now()})
			fmt.Fprintf(c.Writer, "event: heartbeat\ndata: %s\n\n", payload)
			c.Writer.Flush()
		}
	}
}

func sendSnapshot(c *gin.Context, snapshot domain.JourneySnapshot) {
	payload, _ := json.Marshal(snapshot)
	fmt.Fprintf(c.Writer, "event: snapshot\ndata: %s\n\n", payload)
	c.Writer.Flush()
}

// ---------- 夹具 ----------

// finish 延迟一段时间后把快照从 processing 切成 ready 或 failed，
// 让前端能同时验证加载态、完成态和失败态。
func finish(journey domain.Journey, progress domain.JourneyProgress) {
	time.Sleep(analyzeDelay)

	var snapshot domain.JourneySnapshot
	switch strings.ToUpper(journey.Flights[0].Number) {
	case "FAIL":
		msg := "行动决策 Agent 调用失败"
		snapshot = domain.JourneySnapshot{
			Status:  domain.StatusFailed,
			Journey: journey,
			State:   stampedState(domain.NewState()),
			Advice:  domain.NewAdvice(),
			Error:   &msg,
		}
	case "MU9999":
		snapshot = readySnapshot(journey, progress, domain.RiskOrange, domain.FlightStatusDelayed, true)
	default:
		snapshot = readySnapshot(journey, progress, domain.RiskYellow, domain.FlightStatusOnTime, false)
	}

	put(journey.ID, &record{snapshot: snapshot, progress: progress})
}

func processingSnapshot(journey domain.Journey) domain.JourneySnapshot {
	advice := domain.NewAdvice()
	advice.Reasons = []string{"分析进行中，暂无建议"}

	return domain.JourneySnapshot{
		Status:  domain.StatusProcessing,
		Journey: journey,
		State:   stampedState(domain.NewState()),
		Advice:  advice,
	}
}

func readySnapshot(
	journey domain.Journey,
	progress domain.JourneyProgress,
	risk string,
	flightStatus string,
	disrupted bool,
) domain.JourneySnapshot {
	gate := "B27"
	if disrupted {
		gate = "C12"
	}

	hasLocation := progress.HasLocation()

	// 没有定位就算不出路程，相关字段保持 null。
	// 注意口径：缺一项 ≠ 什么都不知道 —— 航班状态和起飞时间都在，
	// 只压到 yellow（与后端的 enforceRiskFloor 一致），不是 unknown。
	if !hasLocation {
		risk = domain.RiskYellow
	}

	state := domain.NewState()
	state.FlightStatus = flightStatus
	state.Gate = ptr(gate)
	state.Timeline = []domain.TimelineNode{
		{Label: "开始登机", Time: "2026-09-11T14:20:00+08:00"},
		{Label: "登机口关闭", Time: "2026-09-11T14:45:00+08:00"},
		{Label: "起飞", Time: "2026-09-11T15:00:00+08:00"},
	}
	state.Guide = []string{
		"T2 入口 → 值机柜台，约 4 分钟",
		"值机柜台 → 安检，约 6 分钟",
		"安检 → " + gate + " 登机口，约 14 分钟",
	}
	if hasLocation {
		state.ETAMin = ptr(52)
		state.Traffic = ptr(domain.TrafficHeavy)
		state.Quality = domain.QualityComplete
	} else {
		state.Quality = domain.QualityDegraded
	}
	state = stampedState(state)

	cards := []domain.Card{{Label: "安检排队", Value: "18 分钟"}}
	alert := "安检排队上升到 45 分钟，缓冲可能不足"
	reasons := []string{
		"航班数据更新于 " + time.Now().Format("15:04") + "，来源 flight_status",
		"安检排队为估计值，置信度低",
	}

	actions := []domain.Action{{
		ID:     "leave_now",
		Title:  "尽快出发",
		Detail: "当前路况拥堵，预计 52 分钟到达机场",
		Nav: &domain.Nav{
			App: "amapuri://route/plan?dlat=31.1443&dlon=121.8083&dname=PVG%20T2",
			Web: "https://uri.amap.com/navigation?to=31.1443,121.8083,PVG%20T2&mode=car",
		},
	}}

	if hasLocation {
		cards = append([]domain.Card{{Label: "预计到达机场", Value: "52 分钟"}}, cards...)
		cards = append(cards,
			domain.Card{Label: "剩余缓冲", Value: "18 分钟"},
			domain.Card{Label: "最晚出发", Value: "13:28"},
		)
	} else {
		alert = "尚未获取定位，无法计算路程时间"
		reasons = append(reasons, "缺少旅客位置，无法计算到机场的耗时")
		actions = []domain.Action{{
			ID:     "enable_location",
			Title:  "开启定位或手动确认位置",
			Detail: "获取位置后才能判断是否来得及",
		}}
	}

	if disrupted {
		actions = append(actions, domain.Action{
			ID:     "contact_airline",
			Title:  "联系航空公司确认",
			Detail: "航班延误且登机口变更，请以航司与机场现场信息为准",
		})
	}

	// 阶段：手动确认优先，其次是"已拿到定位"推断出的在路上
	stage := domain.StageUnknown
	switch {
	case progress.ManualStage != "":
		stage = progress.ManualStage
		reasons = append(reasons, "当前阶段由旅客手动确认，上报定位后自动解除")
	case hasLocation:
		stage = domain.StageEnRoute
	}

	return domain.JourneySnapshot{
		Status:  domain.StatusReady,
		Journey: journey,
		State:   state,
		Advice: domain.Advice{
			Stage:   stage,
			Risk:    risk,
			Alert:   ptr(alert),
			Cards:   cards,
			Actions: actions,
			Reasons: reasons,
		},
	}
}

func stampedState(state domain.State) domain.State {
	state.UpdatedAt = now()
	return state
}

// ---------- 错误响应 ----------

func badRequest(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": msg})
}

func notFound(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "journey not found"})
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	v1 := r.Group("/api/v1")
	v1.POST("/journey", create)
	v1.POST("/journey/:id/location", updateLocation)
	v1.GET("/journey/:id/state", getState)
	v1.GET("/journey/:id/stream", stream)

	log.Printf("mock server listening on %s（仅用于前端联调，不含真实数据）", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}
