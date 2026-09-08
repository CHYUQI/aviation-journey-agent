package api

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *JourneyHandler) Stream(c *gin.Context) {
	if _, err := h.runtime.GetSnapshot(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.SSEvent("snapshot", mustSnapshot(h, c.Param("id")))

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	c.Stream(func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		case <-ticker.C:
			latest, err := h.runtime.GetSnapshot(c.Param("id"))
			if err != nil {
				_, _ = fmt.Fprint(w, ": heartbeat\n\n")
				return true
			}
			c.SSEvent("snapshot", latest)
			return true
		}
	})
}

func mustSnapshot(h *JourneyHandler, id string) interface{} {
	snapshot, _ := h.runtime.GetSnapshot(id)
	return snapshot
}
