package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/runtime"
)

type JourneyHandler struct{ runtime *runtime.Runtime }

func NewJourneyHandler(r *runtime.Runtime) *JourneyHandler { return &JourneyHandler{runtime: r} }

// Create 创建行程。
//
// 返回 202 而不是 200：分析要调用大模型和外部数据技能，可能十几秒，
// 不能阻塞 HTTP 请求。客户端拿到 journeyId 后通过 SSE 或 /state 取结果。
func (h *JourneyHandler) Create(c *gin.Context) {
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
		ID:         newJourneyID(req.Flights[0].Number),
		Flights:    req.Flights,
		HasBaggage: req.HasBaggage,
	}
	if err := h.runtime.CreateJourney(journey); err != nil {
		serverError(c, err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"journeyId": journey.ID,
		"status":    domain.StatusProcessing,
	})
}

// UpdateLocation 上报定位坐标或手动确认阶段，同样返回 202。
func (h *JourneyHandler) UpdateLocation(c *gin.Context) {
	var req domain.UpdateLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请求体格式错误")
		return
	}
	if err := req.Validate(); err != nil {
		badRequest(c, err.Error())
		return
	}

	id := c.Param("id")
	if err := h.runtime.ApplyLocation(id, req); err != nil {
		notFound(c)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"journeyId": id,
		"status":    domain.StatusProcessing,
	})
}

// State 返回当前快照，是前端的唯一状态来源。
func (h *JourneyHandler) State(c *gin.Context) {
	snapshot, err := h.runtime.GetSnapshot(c.Param("id"))
	if err != nil {
		notFound(c)
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

// newJourneyID 生成行程 ID。契约规定它是不透明字符串，客户端不得解析其结构。
func newJourneyID(flightNo string) string {
	buf := make([]byte, 2)
	_, _ = rand.Read(buf)
	return "j_" + strings.ToLower(flightNo) + "_" + hex.EncodeToString(buf)
}
