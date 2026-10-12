package skill

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	// routeETARoadFactor 把直线距离折算成道路距离。
	// 这是保守估计，不是实时导航结果。
	routeETARoadFactor = 1.3
	// routeETAAverageSpeedKmH 是城市驾车平均速度假设。
	// 没有实时路况数据时，用这个固定速度做保守估算。
	routeETAAverageSpeedKmH = 30.0
)

// MaxRouteETAMinutes 是路程时间的可信上限（24 小时）。
//
// 超过它就说明输入本身有问题（定位坐标越界、把地球另一端的坐标传了进来），
// 或者这个行程根本不该用"驾车去机场"来建议。宁可没有数字，
// 也不能把 3 万分钟这种看着精确、实际荒谬的值推给模型和界面 ——
// 实测它会把"还剩 1219 分钟"和"你已严重迟到"同时写出来。
const MaxRouteETAMinutes = 24 * 60

// ValidRouteETAMinutes 判断一个路程时间是否可信。
// DataAgent 在写入 State.ETAMin 之前会再校验一次（技能可能来自别处）。
func ValidRouteETAMinutes(minutes int) bool {
	return minutes >= 1 && minutes <= MaxRouteETAMinutes
}

// RouteETASkill 估算当前位置到出发机场的驾车耗时。
//
// 它不调用外部路径规划 API：
//   - 机场坐标来自 EOOB 机场页；
//   - 当前位置来自前端回传的 Location；
//   - 用 Haversine 直线距离 × 路网系数 ÷ 平均速度估算。
//
// 结果必须标注为估算值；实时路况没有数据源，traffic 保持 null。
type RouteETASkill struct{}

func (RouteETASkill) Name() string { return "route_eta" }

func (RouteETASkill) Supports(field string) bool {
	return field == FieldTravelETA
}

func (RouteETASkill) Execute(ctx context.Context, query Query) (Result, error) {
	if len(query.Journey.Flights) == 0 {
		return Result{Issues: []string{"行程里没有航段，无法估算路程时间"}}, nil
	}
	if query.Location == nil {
		return Result{Issues: []string{"缺少定位，无法估算路程时间"}}, nil
	}

	from := strings.ToUpper(strings.TrimSpace(query.Journey.Flights[0].From))
	if !validAirportIATA(from) {
		return Result{Issues: []string{fmt.Sprintf("出发机场代码 %q 不合法，无法估算路程时间", from)}}, nil
	}

	airportLat, airportLng, err := resolveEOOBAirportCoordinates(ctx, from)
	if err != nil {
		return Result{Issues: []string{"无法获取出发机场坐标：" + err.Error()}}, nil
	}

	etaMinutes, err := estimateDrivingMinutes(
		query.Location.Lat, query.Location.Lng,
		airportLat, airportLng,
	)
	if err != nil {
		// 宁可 etaMin = null + 写明原因，也不给一个不可信的分钟数：
		// 它会被模型当成事实，算出"已经迟到多少分钟"。
		return Result{Issues: []string{"无法估算到机场的路程时间：" + err.Error()}}, nil
	}

	return Result{
		Observations: []Observation{{
			Field:      FieldTravelETA,
			Value:      etaMinutes,
			Source:     "RouteETA 估算（Haversine + 路网系数 + 平均速度）",
			ObservedAt: time.Now().Format(time.RFC3339),
			Confidence: "low",
		}},
		Issues: []string{"路线时间为估算值：直线距离×1.3、按30km/h驾车速度计算，不是实时导航；实时路况未接入，traffic 保持 null"},
	}, nil
}

// estimateDrivingMinutes 估算驾车分钟数。
//
// 返回 error 表示输入或中间结果不可信（坐标越界、NaN/Inf、超出可信上限），
// 调用方必须如实上报，不能退而求其次拿一个数字顶上。
func estimateDrivingMinutes(fromLat, fromLng, toLat, toLng float64) (int, error) {
	if !validCoordinate(fromLat, fromLng) {
		return 0, fmt.Errorf("定位坐标超出取值范围（lat=%g lng=%g）", fromLat, fromLng)
	}
	if !validCoordinate(toLat, toLng) {
		return 0, fmt.Errorf("机场坐标超出取值范围（lat=%g lng=%g）", toLat, toLng)
	}

	distanceKm := haversineKm(fromLat, fromLng, toLat, toLng)
	roadKm := distanceKm * routeETARoadFactor
	if math.IsNaN(distanceKm) || math.IsInf(distanceKm, 0) || distanceKm < 0 ||
		math.IsNaN(roadKm) || math.IsInf(roadKm, 0) {
		return 0, fmt.Errorf("两点距离无法计算（lat=%g lng=%g）", fromLat, fromLng)
	}

	minutes := int(math.Ceil(roadKm / routeETAAverageSpeedKmH * 60))
	if minutes > MaxRouteETAMinutes {
		return 0, fmt.Errorf("直线距离约 %.0f 公里，驾车估算超过 %d 小时，超出可用范围",
			distanceKm, MaxRouteETAMinutes/60)
	}
	if minutes < 1 {
		return 1, nil
	}
	return minutes, nil
}

// validCoordinate 判断经纬度是否在合法范围内，并挡住 NaN / ±Inf。
func validCoordinate(lat, lng float64) bool {
	if math.IsNaN(lat) || math.IsInf(lat, 0) || math.IsNaN(lng) || math.IsInf(lng, 0) {
		return false
	}
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}

func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKm = 6371.0

	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKm * c
}
