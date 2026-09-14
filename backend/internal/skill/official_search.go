package skill

import (
	"context"
	"errors"
)

// OfficialSearchSkill 检索官方发布的补充信息（如机场公告、航司通知）。
//
// 占位实现：真实数据源尚未接入（见 T6）。
type OfficialSearchSkill struct{}

func (OfficialSearchSkill) Name() string { return "official_search" }

func (OfficialSearchSkill) Supports(field string) bool {
	return field == FieldOfficialNotes
}

func (OfficialSearchSkill) Execute(context.Context, Query) (Result, error) {
	return Result{}, errors.New("官方信息检索尚未接入")
}
