// Command mockserver 是前端联调用的假服务端。
//
// 它不调用大模型、不访问外部数据源、不包含任何真实航班数据，
// 只按 docs/api/openapi.yaml 的结构返回固定夹具，用来验证前端在
// processing / ready / failed 三态下的渲染、SSE 推送，以及手动确认阶段的交互。
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
)

const (
	addr         = ":8081"
	analyzeDelay = 2 * time.Second  // 模拟两个 Agent 的分析耗时
	pollEvery    = 1 * time.Second  // mock 内部检查快照是否变化
	heartbeat    = 15 * time.Second // 无变化时的心跳间隔
)

// ---------- 与 openapi.yaml 对齐的结构。可空字段用指针，未知为 null ----------

type Flight struct {
	Number string `json:"number"`
	Date   string `json:"date"`
	From   string `json:"from"`
	To     string `json:"to"`
}

type Journey struct {
	ID         string   `json:"id"`
	Flights    []Flight `json:"flights"`
	HasBaggage bool     `json:"hasBaggage"`
}

type TimelineNode struct {
	Label string `json:"label"`
	Time  string `json:"time"`
}

type State struct {
	FlightStatus string         `json:"flightStatus"`
	Gate         *string        `json:"gate"`
	Timeline     []TimelineNode `json:"timeline"`
	ETAMin       *int           `json:"etaMin"`
	Traffic      *string        `json:"traffic"`
	Guide        []string       `json:"guide"`
	Quality      string         `json:"quality"`
	UpdatedAt    string         `json:"updatedAt"`
}

type Card struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type Nav struct {
	App string `json:"app"`
	Web string `json:"web"`
}

type Action struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Nav    *Nav   `json:"nav"`
}

type Advice struct {
	Stage   string   `json:"stage"`
	Risk    string   `json:"risk"`
	Alert   *string  `json:"alert"`
	Cards   []Card   `json:"cards"`
	Actions []Action `json:"actions"`
	Reasons []string `json:"reasons"`
}

type Snapshot struct {
	Status  string  `json:"status"`
	Journey Journey `json:"journey"`
	State   State   `json:"state"`
	Advice  Advice  `json:"advice"`
	Error   *string `json:"error,omitempty"`
}

// ---------- 存储 ----------

type record struct {
	snapshot    Snapshot
	manualStage string // 旅客手动确认的阶段，空字符串表示未确认
	hasLocation bool   // 是否收到过定位坐标
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

func now() string { return time.Now().Format(time.RFC3339) }

func ptr[T any](v T) *T { return &v }

// ---------- handler ----------

func create(c *gin.Context) {
	var req struct {
		Flights    []Flight `json:"flights"`
		HasBaggage bool     `json:"hasBaggage"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Flights) == 0 {
		badRequest(c, "flights 至少需要一个航段")
		return
	}
	f := req.Flights[0]
	if f.Number == "" || f.Date == "" || f.From == "" || f.To == "" {
		badRequest(c, "航班号、日期、出发机场、到达机场均为必填项")
		return
	}

	journey := Journey{
		ID:         "j_" + strings.ToLower(f.Number),
		Flights:    req.Flights,
		HasBaggage: req.HasBaggage,
	}

	put(journey.ID, &record{snapshot: processingSnapshot(journey)})
	go finish(journey, "", false)

	c.JSON(http.StatusAccepted, gin.H{"journeyId": journey.ID, "status": "processing"})
}

// updateLocation 同时支持上报定位坐标与手动确认阶段。
func updateLocation(c *gin.Context) {
	id := c.Param("id")
	rec, ok := load(id)
	if !ok {
		notFound(c)
		return
	}

	var req struct {
		Lat   *float64 `json:"lat"`
		Lng   *float64 `json:"lng"`
		Stage *string  `json:"stage"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请求体格式错误")
		return
	}

	if (req.Lat == nil) != (req.Lng == nil) {
		badRequest(c, "lat 与 lng 必须成对出现")
		return
	}
	hasCoord := req.Lat != nil && req.Lng != nil
	if !hasCoord && req.Stage == nil {
		badRequest(c, "至少提供 lat+lng 或 stage 之一")
		return
	}
	if req.Stage != nil && !validManualStage(*req.Stage) {
		badRequest(c, "stage 取值不合法")
		return
	}

	manual := rec.manualStage
	if hasCoord {
		manual = "" // 上报坐标后以定位为准，清除手动确认
	}
	if req.Stage != nil {
		manual = *req.Stage
	}

	hasLoc := rec.hasLocation || hasCoord
	journey := rec.snapshot.Journey

	put(id, &record{
		snapshot:    processingSnapshot(journey),
		manualStage: manual,
		hasLocation: hasLoc,
	})
	go finish(journey, manual, hasLoc)

	c.JSON(http.StatusAccepted, gin.H{"journeyId": id, "status": "processing"})
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

func sendSnapshot(c *gin.Context, s Snapshot) {
	payload, _ := json.Marshal(s)
	fmt.Fprintf(c.Writer, "event: snapshot\ndata: %s\n\n", payload)
	c.Writer.Flush()
}

// ---------- 夹具 ----------

func finish(journey Journey, manual string, hasLocation bool) {
	time.Sleep(analyzeDelay)

	// 用航班号切换场景，方便前端一次性验证三种状态
	switch strings.ToUpper(journey.Flights[0].Number) {
	case "FAIL":
		put(journey.ID, &record{
			snapshot:    failedSnapshot(journey),
			manualStage: manual,
			hasLocation: hasLocation,
		})
	case "MU9999":
		put(journey.ID, &record{
			snapshot:    readySnapshot(journey, "orange", "delayed", manual, hasLocation, true),
			manualStage: manual,
			hasLocation: hasLocation,
		})
	default:
		put(journey.ID, &record{
			snapshot:    readySnapshot(journey, "yellow", "on_time", manual, hasLocation, false),
			manualStage: manual,
			hasLocation: hasLocation,
		})
	}
}

func processingSnapshot(journey Journey) Snapshot {
	return Snapshot{
		Status:  "processing",
		Journey: journey,
		State: State{
			FlightStatus: "unknown",
			Gate:         nil,
			Timeline:     []TimelineNode{},
			ETAMin:       nil,
			Traffic:      nil,
			Guide:        []string{},
			Quality:      "unknown",
			UpdatedAt:    now(),
		},
		Advice: Advice{
			Stage:   "unknown",
			Risk:    "unknown",
			Alert:   nil,
			Cards:   []Card{},
			Actions: []Action{},
			Reasons: []string{"分析进行中，暂无建议"},
		},
	}
}

func failedSnapshot(journey Journey) Snapshot {
	s := processingSnapshot(journey)
	s.Status = "failed"
	s.Advice.Reasons = []string{}
	s.Error = ptr("行动决策 Agent 调用失败")
	return s
}

func readySnapshot(journey Journey, risk, flightStatus, manual string, hasLocation, disrupted bool) Snapshot {
	gate := "B27"
	if disrupted {
		gate = "C12"
	}

	stage := "en_route"
	reasons := []string{
		"航班数据更新于 " + time.Now().Format("15:04") + "，来源 flight_status",
		"安检排队为估计值，置信度低",
	}

	// 手动确认阶段优先于自动判断
	if manual != "" {
		stage = manual
		reasons = append(reasons, "当前阶段由旅客手动确认，上报定位后自动解除")
	}

	quality := "complete"
	cards := []Card{
		{Label: "安检排队", Value: "18 分钟"},
	}
	alert := "安检排队上升到 45 分钟，缓冲可能不足"

	if hasLocation {
		cards = append([]Card{
			{Label: "预计到达机场", Value: "52 分钟"},
		}, cards...)
		cards = append(cards,
			Card{Label: "剩余缓冲", Value: "18 分钟"},
			Card{Label: "最晚出发", Value: "13:28"},
		)
	} else {
		// 没有定位时无法计算路程相关指标
		stage = fallbackStage(manual)
		risk = "unknown"
		quality = "degraded"
		alert = "尚未获取定位，无法计算路程时间"
		reasons = append(reasons, "缺少旅客位置，无法计算到机场的耗时")
	}

	actions := []Action{{
		ID:     "leave_now",
		Title:  "尽快出发",
		Detail: "当前路况拥堵，预计 52 分钟到达机场",
		Nav: &Nav{
			App: "amapuri://route/plan?dlat=31.1443&dlon=121.8083&dname=PVG%20T2",
			Web: "https://uri.amap.com/navigation?to=31.1443,121.8083,PVG%20T2&mode=car",
		},
	}}
	if disrupted {
		actions = append(actions, Action{
			ID:     "contact_airline",
			Title:  "联系航空公司确认",
			Detail: "航班延误且登机口变更，请以航司与机场现场信息为准",
			Nav:    nil,
		})
	}
	if !hasLocation {
		actions = []Action{{
			ID:     "enable_location",
			Title:  "开启定位或手动确认位置",
			Detail: "获取位置后才能判断是否来得及",
			Nav:    nil,
		}}
	}

	// 没有定位就算不出路程，对应字段保持 null
	var eta *int
	var traffic *string
	if hasLocation {
		eta = ptr(52)
		traffic = ptr("heavy")
	}

	return Snapshot{
		Status:  "ready",
		Journey: journey,
		State: State{
			FlightStatus: flightStatus,
			Gate:         ptr(gate),
			Timeline: []TimelineNode{
				{Label: "开始登机", Time: "2026-09-11T14:20:00+08:00"},
				{Label: "登机口关闭", Time: "2026-09-11T14:45:00+08:00"},
				{Label: "起飞", Time: "2026-09-11T15:00:00+08:00"},
			},
			ETAMin:    eta,
			Traffic:   traffic,
			Guide:     guideFor(gate),
			Quality:   quality,
			UpdatedAt: now(),
		},
		Advice: Advice{
			Stage:   stage,
			Risk:    risk,
			Alert:   ptr(alert),
			Cards:   cards,
			Actions: actions,
			Reasons: reasons,
		},
	}
}

// fallbackStage：没有定位且旅客没手动确认时，阶段无法判断
func fallbackStage(manual string) string {
	if manual != "" {
		return manual
	}
	return "unknown"
}

func guideFor(gate string) []string {
	return []string{
		"T2 入口 → 值机柜台，约 4 分钟",
		"值机柜台 → 安检，约 6 分钟",
		"安检 → " + gate + " 登机口，约 14 分钟",
	}
}

var manualStages = map[string]bool{
	"en_route":   true,
	"at_airport": true,
	"check_in":   true,
	"security":   true,
	"waiting":    true,
	"boarding":   true,
}

func validManualStage(s string) bool { return manualStages[s] }

// ---------- 错误响应 ----------

func badRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, gin.H{"error": msg})
}

func notFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": "journey not found"})
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
