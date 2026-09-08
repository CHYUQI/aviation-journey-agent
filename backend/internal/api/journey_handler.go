package api

import (
	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/runtime"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

type JourneyHandler struct{ runtime *runtime.Runtime }

func NewJourneyHandler(r *runtime.Runtime) *JourneyHandler { return &JourneyHandler{runtime: r} }
func (h *JourneyHandler) Create(c *gin.Context) {
	var journey domain.Journey
	if err := c.ShouldBindJSON(&journey); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if journey.ID == "" {
		journey.ID = time.Now().Format("20060102150405")
	}
	journey.CreatedAt = time.Now()
	snapshot, err := h.runtime.CreateJourney(c.Request.Context(), journey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, snapshot)
}
func (h *JourneyHandler) UpdateLocation(c *gin.Context) {
	var location domain.Location
	if err := c.ShouldBindJSON(&location); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	location.UpdatedAt = time.Now()
	if err := h.runtime.UpdateLocation(c.Param("id"), location); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	snapshot, err := h.runtime.Recalculate(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, snapshot)
}
func (h *JourneyHandler) State(c *gin.Context) {
	snapshot, err := h.runtime.GetSnapshot(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, snapshot)
}
