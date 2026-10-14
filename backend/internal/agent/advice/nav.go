package advice

import (
	"net/url"
	"strconv"

	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/geo"
)

// navSource 是拼进链接里的调用方标识。
//
// 高德的 URI 接口要求第三方填写来源（App 端叫 sourceApplication，且标注"务必填写"），
// 网页版叫 src。不填不影响跳转，但拿不到任何服务质量保障。
const navSource = "aviation-journey-agent"

// originLabel 是传给高德的起点名称。
// 起点是浏览器定位，不是一个可命名的地点，所以用一个中性说法。
const originLabel = "我的位置"

// BuildAirportNav 生成"导航到机场"的跳转链接。
//
// origin 是旅客当前位置（WGS-84），为空时起点交给高德自己决定。
//
// 两条链接都必须带终点坐标：高德的 URI 以经纬度为核心定位依据，
// 只给地名（dname / 中文名）高德无从确定终点，导航页会是空的。
// 起点的规则两边不同：App 端 slat/slon 是选填（不填自动用当前位置），
// 网页版 from 留空只在移动端自动补，PC 端不会补 —— 所以有定位就显式带上。
//
// 查不到三字码时返回 nil —— 宁可不显示导航按钮，也不要给一个错的目的地。
func BuildAirportNav(iata string, origin *domain.Coordinate) *domain.Nav {
	target, ok := geo.Airport(iata)
	if !ok {
		return nil
	}

	// 高德按 GCJ-02 解释坐标，而基准点与浏览器定位都是 WGS-84，拼链接前统一转一次
	destLng, destLat := geo.WGS84ToGCJ02(target.Lng, target.Lat)
	destName := url.QueryEscape(target.Name)

	// App 端：route/plan 的必填项是 dlat、dlon、t、dev，加上 sourceApplication；
	// dev=0 表示我们给的是已经偏移过的 GCJ-02 坐标。slat/slon 选填。
	app := "amapuri://route/plan/?sourceApplication=" + navSource +
		"&dlat=" + coord(destLat) +
		"&dlon=" + coord(destLng) +
		"&dname=" + destName +
		"&dev=0&t=0"

	// 网页版：from / to 的格式都是 lon,lat[,name]，经度在前。
	// callnative=0：App 唤起已经由前端先试过一次，这里不再重复调起。
	web := "https://uri.amap.com/navigation?mode=car&callnative=0&src=" + navSource +
		"&to=" + coord(destLng) + "," + coord(destLat) + "," + destName

	if origin != nil {
		originLng, originLat := geo.WGS84ToGCJ02(origin.Lng, origin.Lat)
		label := url.QueryEscape(originLabel)

		app += "&slat=" + coord(originLat) +
			"&slon=" + coord(originLng) +
			"&sname=" + label

		web += "&from=" + coord(originLng) + "," + coord(originLat) + "," + label
	}

	return &domain.Nav{App: app, Web: web}
}

// coord 把坐标格式化成高德要求的形式：定点表示、小数点后 6 位（约 0.1 米）。
//
// 不能直接用 %v：那会输出 121.805 这种位数不足的值，也容易掉进科学计数法。
func coord(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

// navActionID 是"导航到机场"这条工具行动的标识。
const navActionID = "navigate_airport"

// navNeededStages 是"还需要去机场"的阶段。
//
// 用白名单而不是黑名单：多给一个用不上的导航按钮，比在人已经进了机场
// （甚至飞机已经起飞）时还给一个"导航到机场"要好。
var navNeededStages = map[string]bool{
	domain.StageUnknown: true, // 阶段未知多半是还没到机场
	domain.StageEnRoute: true,
}

// ensureAirportNav 保证"有目的地 + 有起点就能打开地图"这条工具链路始终可用。
//
// 导航是工具动作，不是风险结论：它只回答"怎么去机场"，不回答"来不来得及"。
// 所以它必须在 enforceRiskFloor 之后补 —— 那一步在关键数据缺失时会把风险降到
// unknown，并撤掉所有带导航的行动（它针对的是"立即出发"这类带判断的建议）。
// 但"打开地图"这件事不该跟着一起消失：恰恰是数据不全的时候，旅客更需要自己看路况。
//
// 模型已经给出带导航的行动时不重复添加。
func ensureAirportNav(result *domain.Advice, in Input) {
	// 没有起点就不给。网页版的 from 留空只在移动端自动补当前位置，
	// PC 端不会补 —— 没有定位就给链接，旅客会看到两个空框。
	if in.Progress.Location == nil {
		return
	}
	if !navNeededStages[result.Stage] {
		return
	}
	for _, a := range result.Actions {
		if a.Nav != nil {
			return
		}
	}

	target, ok := geo.Airport(in.AirportIATA)
	if !ok {
		return
	}
	nav := BuildAirportNav(in.AirportIATA, in.Progress.Location)
	if nav == nil {
		return
	}

	result.Actions = append(result.Actions, domain.Action{
		ID:     navActionID,
		Title:  "导航到机场",
		Detail: "从当前位置到" + target.Name + "的驾车路线",
		Nav:    nav,
	})
}
