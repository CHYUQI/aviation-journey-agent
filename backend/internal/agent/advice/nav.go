package advice

import (
	"net/url"
	"strings"

	"aviation-journey-agent/backend/internal/domain"
)

// airportNames 是三字码到机场中文名的对照表。
//
// 这是一份静态资料，不是地图 API：我们只按高德的 URI 规则拼一个跳转链接，
// 由旅客自己决定点不点。没有任何地图数据接口调用，也没有地图 SDK。
var airportNames = map[string]string{
	"PEK": "北京首都国际机场",
	"PKX": "北京大兴国际机场",
	"PVG": "上海浦东国际机场",
	"SHA": "上海虹桥国际机场",
	"CAN": "广州白云国际机场",
	"SZX": "深圳宝安国际机场",
	"CTU": "成都双流国际机场",
	"TFU": "成都天府国际机场",
	"CKG": "重庆江北国际机场",
	"XIY": "西安咸阳国际机场",
	"HGH": "杭州萧山国际机场",
	"NKG": "南京禄口国际机场",
	"WUH": "武汉天河国际机场",
	"CSX": "长沙黄花国际机场",
	"TAO": "青岛胶东国际机场",
	"TSN": "天津滨海国际机场",
	"KMG": "昆明长水国际机场",
	"XMN": "厦门高崎国际机场",
	"DLC": "大连周水子国际机场",
	"SHE": "沈阳桃仙国际机场",
}

// BuildAirportNav 生成"导航到机场"的跳转链接。
//
// 目的地用机场中文名而不是经纬度，这样不需要任何坐标服务。
// app 优先唤起高德 App，装不上时回退网页版。
// 查不到三字码时返回 nil —— 宁可不显示导航按钮，也不要给一个错的目的地。
func BuildAirportNav(iata string) *domain.Nav {
	name, ok := airportNames[strings.ToUpper(strings.TrimSpace(iata))]
	if !ok {
		return nil
	}

	dest := url.QueryEscape(name)
	return &domain.Nav{
		App: "amapuri://route/plan?dname=" + dest + "&mode=car",
		Web: "https://uri.amap.com/navigation?dname=" + dest + "&mode=car",
	}
}
