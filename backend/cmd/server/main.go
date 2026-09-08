package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"aviation-journey-agent/backend/internal/agent/advice"
	dataagent "aviation-journey-agent/backend/internal/agent/data"
	"aviation-journey-agent/backend/internal/api"
	"aviation-journey-agent/backend/internal/runtime"
	"aviation-journey-agent/backend/internal/skill"
	"aviation-journey-agent/backend/internal/store"
)

func main() {
	memoryStore := store.NewMemoryStore()
	skillRegistry := skill.NewRegistry()
	skillRegistry.Register(skill.FlightStatusSkill{})
	skillRegistry.Register(skill.AirportStatusSkill{})
	skillRegistry.Register(skill.RouteETASkill{})
	skillRegistry.Register(skill.OfficialSearchSkill{})
	dataAgent := dataagent.NewAgent(skillRegistry)
	adviceAgent := advice.NewAgent()
	journeyRuntime := runtime.New(memoryStore, dataAgent, adviceAgent)

	router := gin.Default()
	api.RegisterRoutes(router, journeyRuntime)

	log.Println("aviation-journey-agent backend listening on :8080")
	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
