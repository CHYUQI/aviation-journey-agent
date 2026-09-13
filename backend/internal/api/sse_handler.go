package api

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"aviation-journey-agent/backend/internal/domain"
)

const (
	sseHeartbeatInterval = 15 * time.Second
	sseRetryHint         = 5000
)

// Stream 是 SSE 实时推送，结构见契约 3.4。
//
// 推送规则：
//  1. 连接建立后立刻推一条 snapshot。
//  2. 快照一变就推（位置或阶段更新后立即推，不等周期刷新）。
//  3. 无变化时每 15 秒推一条 heartbeat。
//
// 快照是全量覆盖语义，丢了中间某帧也不会导致状态错乱。
func (h *JourneyHandler) Stream(c *gin.Context) {
	id := c.Param("id")

	// 先订阅再读快照，避免两者之间发生的更新被漏掉
	updates, unsubscribe := h.runtime.Subscribe(id)
	defer unsubscribe()

	snapshot, err := h.runtime.GetSnapshot(id)
	if err != nil {
		notFound(c)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	_, _ = fmt.Fprintf(c.Writer, "retry: %d\n\n", sseRetryHint)
	c.Writer.Flush()

	writeSnapshot(c, snapshot)

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case next, ok := <-updates:
			if !ok {
				return
			}
			writeSnapshot(c, next)
		case <-heartbeat.C:
			payload, _ := json.Marshal(gin.H{"at": time.Now().Format(time.RFC3339)})
			_, _ = fmt.Fprintf(c.Writer, "event: heartbeat\ndata: %s\n\n", payload)
			c.Writer.Flush()
		}
	}
}

func writeSnapshot(c *gin.Context, snapshot domain.JourneySnapshot) {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(c.Writer, "event: snapshot\ndata: %s\n\n", payload)
	c.Writer.Flush()
}
