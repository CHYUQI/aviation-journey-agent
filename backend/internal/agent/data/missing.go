package data

import "aviation-journey-agent/backend/internal/domain"

func MissingFields(state domain.WorldState) []string {
	fields := make([]string, 0)
	if state.Flight.Status == "" {
		fields = append(fields, "flight.status")
	}
	if state.Airport.Code == "" {
		fields = append(fields, "airport.status")
	}
	if state.Travel.ETAMin == 0 {
		fields = append(fields, "travel.eta")
	}
	return fields
}
