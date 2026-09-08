package data

import (
	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/skill"
	"context"
	"time"
)

type Agent struct{ skills *skill.Registry }

func NewAgent(skills *skill.Registry) *Agent { return &Agent{skills: skills} }

func (a *Agent) BuildState(ctx context.Context, journey domain.Journey, previous *domain.WorldState) (domain.WorldState, error) {
	state := domain.WorldState{
		Passenger: domain.PassengerState{HasBaggage: journey.Passenger.HasBaggage},
		Evidence:  []domain.Evidence{}, DataQuality: domain.DataQuality{Status: "degraded"}, GeneratedAt: time.Now(),
	}
	if len(journey.Flights) > 0 {
		state.Flight.Number = journey.Flights[0].Number
	}
	// TODO: 检查缺失/过期字段，调用真实 Skill，将 Observation 标准化为 WorldState。
	if a.skills == nil {
		state.DataQuality.Issues = append(state.DataQuality.Issues, "skill registry is nil")
	} else {
		result, _ := a.skills.Query(ctx, skill.Query{Journey: journey, Previous: previous, Fields: []string{"flight.status", "airport.status", "travel.eta"}})
		state.DataQuality.Issues = append(state.DataQuality.Issues, result.Issues...)
	}
	return state, nil
}
