// Package geo 提供坐标转换与机场坐标表。
//
// 两个坐标来源：浏览器定位（WGS-84）与机场基准点（WGS-84，民航标准）。
// 高德的 URI 与 REST API 都按 GCJ-02 解释坐标，调用前必须统一转一次。
package geo

import "math"

// Krassovsky 椭球参数，与高德公开的 GCJ-02 偏移公式一致。
const (
	krassovskyA  = 6378245.0
	krassovskyEE = 0.00669342162296594323
)

// WGS84ToGCJ02 把 WGS-84 坐标转成 GCJ-02。
//
// 入参与返回都是 (经度, 纬度)。境外坐标不偏移。
// 这是拟合公式，误差米级，对"到机场的导航/路线规划"足够。
func WGS84ToGCJ02(lng, lat float64) (float64, float64) {
	if outsideChina(lng, lat) {
		return lng, lat
	}

	dLat := offsetLat(lng-105.0, lat-35.0)
	dLng := offsetLng(lng-105.0, lat-35.0)

	radLat := lat / 180.0 * math.Pi
	magic := math.Sin(radLat)
	magic = 1 - krassovskyEE*magic*magic
	sqrtMagic := math.Sqrt(magic)

	dLat = (dLat * 180.0) / ((krassovskyA * (1 - krassovskyEE)) / (magic * sqrtMagic) * math.Pi)
	dLng = (dLng * 180.0) / (krassovskyA / sqrtMagic * math.Cos(radLat) * math.Pi)

	return lng + dLng, lat + dLat
}

func outsideChina(lng, lat float64) bool {
	return lng < 72.004 || lng > 137.8347 || lat < 0.8293 || lat > 55.8271
}

func offsetLat(x, y float64) float64 {
	ret := -100.0 + 2.0*x + 3.0*y + 0.2*y*y + 0.1*x*y + 0.2*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(y*math.Pi) + 40.0*math.Sin(y/3.0*math.Pi)) * 2.0 / 3.0
	ret += (160.0*math.Sin(y/12.0*math.Pi) + 320.0*math.Sin(y*math.Pi/30.0)) * 2.0 / 3.0
	return ret
}

func offsetLng(x, y float64) float64 {
	ret := 300.0 + x + 2.0*y + 0.1*x*x + 0.1*x*y + 0.1*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(x*math.Pi) + 40.0*math.Sin(x/3.0*math.Pi)) * 2.0 / 3.0
	ret += (150.0*math.Sin(x/12.0*math.Pi) + 300.0*math.Sin(x/30.0*math.Pi)) * 2.0 / 3.0
	return ret
}
