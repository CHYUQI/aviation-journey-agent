package skill

import (
	"context"
	"errors"
)

// AirportStatusSkill 查询机场信息：航站楼、安检排队、机场内步行指引。
//
// 占位实现：真实数据源尚未接入（见 T6）。
type AirportStatusSkill struct{}

func (AirportStatusSkill) Name() string { return "airport_status" }

func (AirportStatusSkill) Supports(field string) bool {
	return field == FieldAirportGuide
}

func (AirportStatusSkill) Execute(context.Context, Query) (Result, error) {
	return Result{}, errors.New("机场数据源尚未接入")
}
