package domain

import "errors"

// Journey 是旅客提交的行程。对应契约 schemas.Journey。
type Journey struct {
	ID         string   `json:"id"`
	Flights    []Flight `json:"flights"`
	HasBaggage bool     `json:"hasBaggage"`
}

// Flight 是一个航段。对应契约 schemas.Flight。
type Flight struct {
	Number string `json:"number"` // 航班号，大写不含空格
	Date   string `json:"date"`   // YYYY-MM-DD
	From   string `json:"from"`   // IATA 三字码
	To     string `json:"to"`     // IATA 三字码
}

// CreateJourneyRequest 是 POST /journey 的请求体。
type CreateJourneyRequest struct {
	Flights    []Flight `json:"flights"`
	HasBaggage bool     `json:"hasBaggage"`
}

// Validate 校验创建行程请求。返回的文案可直接作为 error 字段返回。
func (r CreateJourneyRequest) Validate() error {
	if len(r.Flights) == 0 {
		return errors.New("flights 至少需要一个航段")
	}
	for _, f := range r.Flights {
		if f.Number == "" || f.Date == "" || f.From == "" || f.To == "" {
			return errors.New("航班号、日期、出发机场、到达机场均为必填项")
		}
	}
	return nil
}

// UpdateLocationRequest 是 POST /journey/{id}/location 的请求体。
//
// 同一个接口承载两种上报：
//   - 上报定位坐标：给 lat + lng
//   - 手动确认阶段：给 stage
//
// 至少提供其一；lat 与 lng 必须成对出现。
type UpdateLocationRequest struct {
	Lat   *float64 `json:"lat"`
	Lng   *float64 `json:"lng"`
	Stage *string  `json:"stage"`
}

// HasCoordinate 表示提供了成对的坐标。
func (r UpdateLocationRequest) HasCoordinate() bool { return r.Lat != nil && r.Lng != nil }

// HasPartialCoordinate 表示只给了一半坐标。
func (r UpdateLocationRequest) HasPartialCoordinate() bool {
	return (r.Lat == nil) != (r.Lng == nil)
}

// Validate 校验定位/阶段上报请求。
func (r UpdateLocationRequest) Validate() error {
	if r.HasPartialCoordinate() {
		return errors.New("lat 与 lng 必须成对出现")
	}
	if !r.HasCoordinate() && r.Stage == nil {
		return errors.New("至少提供 lat+lng 或 stage 之一")
	}
	if r.Stage != nil && !IsManualStage(*r.Stage) {
		return errors.New("stage 取值不合法")
	}
	return nil
}

// Coordinate 是旅客位置。仅在后端内部使用，不进入接口响应。
type Coordinate struct {
	Lat float64
	Lng float64
}

// JourneyProgress 是契约之外的内部状态，不参与任何序列化。
//
// 它记录旅客当前位置与手动确认的阶段。手动确认的阶段会一直生效，
// 直到下一次上报坐标（见 UpdateLocationRequest 的处理规则）。
type JourneyProgress struct {
	Location    *Coordinate
	ManualStage string // 空字符串表示未手动确认
}

// HasLocation 表示当前是否持有有效位置。
func (p JourneyProgress) HasLocation() bool { return p.Location != nil }
