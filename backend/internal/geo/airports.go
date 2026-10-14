package geo

// AirportInfo 是一个机场的基准坐标与中文名。
type AirportInfo struct {
	Name string
	Lat  float64 // WGS-84 机场基准点
	Lng  float64
}

// Airport 按 IATA 三字码查机场基准坐标（WGS-84）。未收录返回 ok=false。
//
// 坐标来源：Great Circle Mapper 中国大型国际机场坐标表（机场基准点，WGS-84）；
// 昆明长水另取 ZPPP 航图（GCM 那行仍是 2012 年停用的巫家坝老机场，不能用）。
//
// 口径：基准点由 MH/T 4015 定义为机场的几何中心，与具体航站楼入口可能相差
// 1~2 公里。要精确到航站楼入口就得接地图 POI 服务，与项目约定冲突，
// 所以整张表统一用基准点，口径一致、可复核。
func Airport(iata string) (AirportInfo, bool) {
	info, ok := airportTable[normalizeIATA(iata)]
	return info, ok
}

func normalizeIATA(iata string) string {
	out := make([]byte, 0, 3)
	for i := 0; i < len(iata) && len(out) < 3; i++ {
		c := iata[i]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		if c >= 'A' && c <= 'Z' {
			out = append(out, c)
		}
	}
	return string(out)
}

var airportTable = map[string]AirportInfo{
	"PEK": {Name: "北京首都国际机场", Lat: 40.080000, Lng: 116.584722},
	"PKX": {Name: "北京大兴国际机场", Lat: 39.509722, Lng: 116.410833},
	"PVG": {Name: "上海浦东国际机场", Lat: 31.143333, Lng: 121.805000},
	"SHA": {Name: "上海虹桥国际机场", Lat: 31.197778, Lng: 121.335833},
	"CAN": {Name: "广州白云国际机场", Lat: 23.392222, Lng: 113.298889},
	"SZX": {Name: "深圳宝安国际机场", Lat: 22.639167, Lng: 113.810833},
	"CTU": {Name: "成都双流国际机场", Lat: 30.578333, Lng: 103.946944},
	"TFU": {Name: "成都天府国际机场", Lat: 30.312500, Lng: 104.441111},
	"CKG": {Name: "重庆江北国际机场", Lat: 29.719167, Lng: 106.641944},
	"XIY": {Name: "西安咸阳国际机场", Lat: 34.446944, Lng: 108.751944},
	"HGH": {Name: "杭州萧山国际机场", Lat: 30.229444, Lng: 120.433889},
	"NKG": {Name: "南京禄口国际机场", Lat: 31.741944, Lng: 118.861944},
	"WUH": {Name: "武汉天河国际机场", Lat: 30.783611, Lng: 114.207778},
	"CSX": {Name: "长沙黄花国际机场", Lat: 28.189167, Lng: 113.220000},
	"TAO": {Name: "青岛胶东国际机场", Lat: 36.361944, Lng: 120.088056},
	"TSN": {Name: "天津滨海国际机场", Lat: 39.124167, Lng: 117.345833},
	"KMG": {Name: "昆明长水国际机场", Lat: 25.105000, Lng: 102.941667},
	"XMN": {Name: "厦门高崎国际机场", Lat: 24.543889, Lng: 118.127778},
	"DLC": {Name: "大连周水子国际机场", Lat: 38.965556, Lng: 121.538889},
	"SHE": {Name: "沈阳桃仙国际机场", Lat: 41.639722, Lng: 123.482778},
}
