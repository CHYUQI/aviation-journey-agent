package skill

import (
	"context"
	"errors"
)

type FlightStatusSkill struct{}

func (FlightStatusSkill) Name() string               { return "flight_status" }
func (FlightStatusSkill) Supports(field string) bool { return field == "flight.status" }
func (FlightStatusSkill) Execute(context.Context, Query) (Result, error) {
	return Result{}, errors.New("real flight source is not configured")
}
