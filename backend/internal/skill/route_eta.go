package skill

import (
	"context"
	"errors"
)

type RouteETASkill struct{}

func (RouteETASkill) Name() string               { return "route_eta" }
func (RouteETASkill) Supports(field string) bool { return field == "travel.eta" }
func (RouteETASkill) Execute(context.Context, Query) (Result, error) {
	return Result{}, errors.New("real route source is not configured")
}
