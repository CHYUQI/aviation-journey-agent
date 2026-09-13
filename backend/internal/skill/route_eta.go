package skill

import (
	"context"
	"errors"
)

// RouteETASkill 计算当前位置到机场的预计耗时。
//
// 它依赖定位：没有 Location 时应当直接返回空结果，而不是用默认值估算。
// 占位实现：真实数据源尚未接入（见 T6）。
type RouteETASkill struct{}

func (RouteETASkill) Name() string { return "route_eta" }

func (RouteETASkill) Supports(field string) bool {
	return field == FieldTravelETA
}

func (RouteETASkill) Execute(_ context.Context, query Query) (Result, error) {
	if query.Location == nil {
		return Result{Issues: []string{"缺少定位，无法计算路程时间"}}, nil
	}
	return Result{}, errors.New("路径规划数据源尚未接入")
}
