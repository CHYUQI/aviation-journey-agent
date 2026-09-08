package api

import (
	"aviation-journey-agent/backend/internal/runtime"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, journeyRuntime *runtime.Runtime) {
	v1 := router.Group("/api/v1")
	handler := NewJourneyHandler(journeyRuntime)
	v1.POST("/journey", handler.Create)
	v1.POST("/journey/:id/location", handler.UpdateLocation)
	v1.GET("/journey/:id/state", handler.State)
	v1.GET("/journey/:id/stream", handler.Stream)
}
