package skill

import (
	"context"
	"errors"
)

type AirportStatusSkill struct{}

func (AirportStatusSkill) Name() string               { return "airport_status" }
func (AirportStatusSkill) Supports(field string) bool { return field == "airport.status" }
func (AirportStatusSkill) Execute(context.Context, Query) (Result, error) {
	return Result{}, errors.New("real airport source is not configured")
}
