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

	etaMinutes := estimateDrivingMinutes(
		query.Location.Lat, query.Location.Lng,
		airportLat, airportLng,
	)

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

func estimateDrivingMinutes(fromLat, fromLng, toLat, toLng float64) int {
	distanceKm := haversineKm(fromLat, fromLng, toLat, toLng)
	roadKm := distanceKm * routeETARoadFactor
	minutes := int(math.Ceil(roadKm / routeETAAverageSpeedKmH * 60))
	if minutes < 1 {
		return 1
	}
	return minutes
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
