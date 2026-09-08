package skill

import (
	"context"
	"errors"
)

type OfficialSearchSkill struct{}

func (OfficialSearchSkill) Name() string               { return "official_search" }
func (OfficialSearchSkill) Supports(field string) bool { return field == "official.lookup" }
func (OfficialSearchSkill) Execute(context.Context, Query) (Result, error) {
	return Result{}, errors.New("official search source is not configured")
}
