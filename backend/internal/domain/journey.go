package domain

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

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
//
// HasBaggage 用指针：契约里它是 required，而 bool 的零值 false 无法区分
// "旅客没带行李"和"客户端根本没传这个字段"。缺字段必须报 400，不能默认 false。
type CreateJourneyRequest struct {
	Flights    []Flight `json:"flights"`
	HasBaggage *bool    `json:"hasBaggage"`
}

// iataPattern 与契约 schemas.Flight.from/to 的 pattern ^[A-Z]{3}$ 一致。
var iataPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// flightNumberPattern 是航班号的取值口径：两位承运人代码（至少含一个字母，
// 如 CA / 9C / 3U / 8L）+ 1~4 位数字，允许末尾一位字母（如 CA1234A）。
// 与契约描述"统一大写不含空格"一致。
//
// 它同时挡住了把航班号当自由文本用的情况（空格、换行、控制字符、超长串、
// 提示注入段落），因为航班号会被拼进 EOOB 请求和模型提示词。
var flightNumberPattern = regexp.MustCompile(`^(?:[A-Z][A-Z0-9]|[A-Z0-9][A-Z])[0-9]{1,4}[A-Z]?$`)

// Validate 校验创建行程请求。返回的文案可直接作为 error 字段返回。
func (r CreateJourneyRequest) Validate() error {
	if len(r.Flights) == 0 {
		return errors.New("flights 至少需要一个航段")
	}
	if r.HasBaggage == nil {
		return errors.New("hasBaggage 为必填项")
	}
	for i, f := range r.Flights {
		if err := f.Validate(); err != nil {
			return fmt.Errorf("第 %d 个航段：%w", i+1, err)
		}
	}
	return nil
}

// Validate 校验单个航段，约束与 docs/api/openapi.yaml 的 schemas.Flight 一致。
func (f Flight) Validate() error {
	if f.Number == "" || f.Date == "" || f.From == "" || f.To == "" {
		return errors.New("航班号、日期、出发机场、到达机场均为必填项")
	}
	if !flightNumberPattern.MatchString(f.Number) {
		return fmt.Errorf("航班号 %q 不合法（应为大写字母数字，如 CA1234）", f.Number)
	}
	// 契约要求 format: date，这里同时挡住 2026-9-1、2026-13-45 和带时间的字符串。
	if _, err := time.Parse("2006-01-02", f.Date); err != nil {
		return fmt.Errorf("日期 %q 不合法（应为 YYYY-MM-DD）", f.Date)
	}
	if !iataPattern.MatchString(f.From) {
		return fmt.Errorf("出发机场 %q 不合法（应为三位大写字母的 IATA 代码）", f.From)
	}
	if !iataPattern.MatchString(f.To) {
		return fmt.Errorf("到达机场 %q 不合法（应为三位大写字母的 IATA 代码）", f.To)
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
