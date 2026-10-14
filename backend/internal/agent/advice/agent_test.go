package advice

import (
	"context"
	"errors"
	"math"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/geo"
	"aviation-journey-agent/backend/internal/model"
)

type fakeClient struct {
	content string
	err     error
	calls   int
	lastReq model.Request
}

func (f *fakeClient) Chat(_ context.Context, req model.Request) (model.Response, error) {
	f.calls++
	f.lastReq = req
	if f.err != nil {
		return model.Response{}, f.err
	}
	return model.Response{Content: f.content}, nil
}

func ptr[T any](v T) *T { return &v }

// readyState 是一份"数据齐全"的状态：航班状态、时间轴、路程时间都有。
func readyState() domain.State {
	s := domain.NewState()
	s.FlightStatus = domain.FlightStatusOnTime
	s.Gate = ptr("B27")
	s.Timeline = []domain.TimelineNode{
		{Label: "登机口关闭", Time: "2026-09-13T14:45:00+08:00"},
		{Label: "起飞", Time: "2026-09-13T15:00:00+08:00"},
	}
	s.ETAMin = ptr(52)
	s.Traffic = ptr(domain.TrafficHeavy)
	s.Quality = domain.QualityComplete
	return s
}

func TestEvaluate_UsesModelOutputAndInjectsNav(t *testing.T) {
	client := &fakeClient{content: `{
		"stage": "en_route",
		"risk": "yellow",
		"alert": "路况拥堵，建议现在出发",
		"cards": [{"label": "剩余缓冲", "value": "18 分钟"}],
		"actions": [{"id": "leave_now", "title": "尽快出发", "detail": "预计 52 分钟到机场", "nav": "airport"}],
		"reasons": ["航班数据更新于 12:35"]
	}`}

	got := NewAgent(client).Evaluate(context.Background(), Input{
		State:       readyState(),
		AirportIATA: "PVG",
	})

	if got.Stage != domain.StageEnRoute || got.Risk != domain.RiskYellow {
		t.Fatalf("stage/risk = %s/%s", got.Stage, got.Risk)
	}
	if got.Alert == nil || !strings.Contains(*got.Alert, "拥堵") {
		t.Fatalf("alert = %v", got.Alert)
	}
	if len(got.Actions) != 1 {
		t.Fatalf("actions = %d", len(got.Actions))
	}
	// nav 是代码注入的，不是模型给的
	nav := got.Actions[0].Nav
	if nav == nil || !strings.HasPrefix(nav.App, "amapuri://") || !strings.Contains(nav.Web, "uri.amap.com") {
		t.Fatalf("nav 未被正确注入: %+v", nav)
	}
	if !strings.Contains(nav.App, "%E4%B8%8A%E6%B5%B7") { // 上海
		t.Fatalf("目的地应经过 URL 编码: %s", nav.App)
	}
	if len(got.Cards) != 1 || got.Cards[0].Label != "剩余缓冲" {
		t.Fatalf("cards = %+v", got.Cards)
	}
	if client.calls != 1 {
		t.Fatalf("应当只调用一次模型，实际 %d", client.calls)
	}
}

func TestEvaluate_MissingETACannotClaimGreen(t *testing.T) {
	// 模型说"很安全"，但此时没有路程时间，判断不了来不来得及
	client := &fakeClient{content: `{
		"stage": "en_route",
		"risk": "green",
		"alert": null,
		"cards": [],
		"actions": [{"id": "leave_now", "title": "出发", "detail": "现在走", "nav": "airport"}],
		"reasons": []
	}`}

	state := readyState()
	state.ETAMin = nil

	got := NewAgent(client).Evaluate(context.Background(), Input{State: state, AirportIATA: "PVG"})

	// 缺路程时间但航班状态、起飞时间都有：还能做有限判断 → 压到 yellow，不是 unknown
	if got.Risk != domain.RiskYellow {
		t.Fatalf("缺失路程时间时应降级为 yellow，实际 %s", got.Risk)
	}
	if !hasReason(got.Reasons, "路程时间") {
		t.Fatalf("reasons 应说明降级原因: %v", got.Reasons)
	}
	if len(got.Actions) == 0 {
		t.Fatal("yellow 仍可给出行动（例如先出发、开导航确认路况），不应被清空")
	}
}

// TestRiskConstraintFor 锁住两级约束：缺一点 ≠ 什么都不知道。
func TestRiskConstraintFor(t *testing.T) {
	full := readyState()

	t.Run("数据齐全时无约束", func(t *testing.T) {
		if c := RiskConstraintFor(full, domain.StageEnRoute); len(c.Missing) != 0 {
			t.Fatalf("数据齐全不应有约束，实际 %+v", c)
		}
	})

	t.Run("只缺路程时间：不得 green，但不必 unknown", func(t *testing.T) {
		state := full
		state.ETAMin = nil
		c := RiskConstraintFor(state, domain.StageEnRoute)
		if c.Unknown {
			t.Fatal("航班状态与起飞时间都在时，不应要求 unknown")
		}
		if !strings.Contains(strings.Join(c.Missing, "、"), "路程时间") {
			t.Fatalf("应指出缺少路程时间，实际 %v", c.Missing)
		}
	})

	t.Run("只缺航班状态：不得 green", func(t *testing.T) {
		state := full
		state.FlightStatus = domain.FlightStatusUnknown
		if c := RiskConstraintFor(state, domain.StageEnRoute); c.Unknown {
			t.Fatalf("还有起飞时间可判断，不应要求 unknown：%+v", c)
		}
	})

	t.Run("航班状态与时间轴都缺：只能 unknown", func(t *testing.T) {
		state := domain.NewState()
		c := RiskConstraintFor(state, domain.StageUnknown)
		if !c.Unknown {
			t.Fatalf("完全无法判断时应要求 unknown：%+v", c)
		}
	})
}

// TestEvaluate_NoFlightDataAtAllIsUnknown：连航班状态和时间都没有时，
// 即使模型给了 yellow，也必须压成 unknown 并撤掉高强度行动。
func TestEvaluate_NoFlightDataAtAllIsUnknown(t *testing.T) {
	client := &fakeClient{content: `{
		"stage": "unknown",
		"risk": "yellow",
		"alert": null,
		"cards": [],
		"actions": [{"id": "leave_now", "title": "出发", "detail": "现在走", "nav": "airport"}],
		"reasons": []
	}`}

	got := NewAgent(client).Evaluate(context.Background(), Input{State: domain.NewState(), AirportIATA: "PVG"})

	if got.Risk != domain.RiskUnknown {
		t.Fatalf("完全无数据时应为 unknown，实际 %s", got.Risk)
	}
	if len(got.Actions) != 0 {
		t.Fatalf("unknown 时不应保留高强度行动，实际 %d 条", len(got.Actions))
	}
	if !hasReason(got.Reasons, "无法判断风险") {
		t.Fatalf("reasons 应说明降级原因: %v", got.Reasons)
	}
}

func TestEvaluate_RejectsUnknownEnumValues(t *testing.T) {
	client := &fakeClient{content: `{
		"stage": "flying",
		"risk": "green",
		"alert": null,
		"cards": [],
		"actions": [],
		"reasons": []
	}`}

	got := NewAgent(client).Evaluate(context.Background(), Input{State: readyState()})

	if got.Risk != domain.RiskUnknown {
		t.Fatalf("非法枚举应整体降级，实际 risk=%s stage=%s", got.Risk, got.Stage)
	}
	if !hasReason(got.Reasons, "无法识别") {
		t.Fatalf("reasons 应说明原因: %v", got.Reasons)
	}
}

func TestEvaluate_ModelFailureFallsBack(t *testing.T) {
	client := &fakeClient{err: errors.New("connection refused")}

	got := NewAgent(client).Evaluate(context.Background(), Input{State: readyState()})

	if got.Risk != domain.RiskUnknown || len(got.Actions) != 0 {
		t.Fatalf("模型失败时应返回安全兜底，实际 %+v", got)
	}
	if !hasReason(got.Reasons, "调用失败") {
		t.Fatalf("reasons 应说明失败原因: %v", got.Reasons)
	}
}

func TestEvaluate_NilClientStillProducesValidAdvice(t *testing.T) {
	got := NewAgent(nil).Evaluate(context.Background(), Input{State: readyState()})

	if got.Risk != domain.RiskUnknown || got.Stage != domain.StageUnknown {
		t.Fatalf("没有模型时应返回 unknown，实际 risk=%s", got.Risk)
	}
	// 契约要求数组不能是 null
	if got.Cards == nil || got.Actions == nil || got.Reasons == nil {
		t.Fatal("数组字段不能为 nil，序列化后会是 null")
	}
}

func TestEvaluate_ManualStageOverridesModel(t *testing.T) {
	client := &fakeClient{content: `{
		"stage": "en_route",
		"risk": "yellow",
		"alert": null,
		"cards": [],
		"actions": [],
		"reasons": []
	}`}

	state := readyState()
	state.Timeline = []domain.TimelineNode{{Label: "起飞", Time: "2026-09-13T15:00:00+08:00"}}

	got := NewAgent(client).Evaluate(context.Background(), Input{
		State:    state,
		Progress: domain.JourneyProgress{ManualStage: domain.StageSecurity},
	})

	if got.Stage != domain.StageSecurity {
		t.Fatalf("手动确认的阶段应优先，实际 %s", got.Stage)
	}
	if !hasReason(got.Reasons, "手动确认") {
		t.Fatalf("reasons 应说明阶段来源: %v", got.Reasons)
	}
}

func TestEvaluate_UnknownAirportDropsNav(t *testing.T) {
	client := &fakeClient{content: `{
		"stage": "en_route",
		"risk": "yellow",
		"alert": null,
		"cards": [],
		"actions": [{"id": "leave_now", "title": "出发", "detail": "现在走", "nav": "airport"}],
		"reasons": []
	}`}

	got := NewAgent(client).Evaluate(context.Background(), Input{
		State:       readyState(),
		AirportIATA: "ZZZ", // 对照表里没有
	})

	if len(got.Actions) != 1 {
		t.Fatalf("行动应保留，实际 %d 条", len(got.Actions))
	}
	if got.Actions[0].Nav != nil {
		t.Fatalf("查不到机场时不能给导航链接，实际 %+v", got.Actions[0].Nav)
	}
}

func TestBuildAirportNav_UnknownCodeHasNoLink(t *testing.T) {
	if BuildAirportNav("ZZZ", nil) != nil {
		t.Fatal("未知三字码应返回 nil")
	}
}

// 两条链接都必须带终点坐标。
//
// 高德的 URI 以经纬度定位目的地：只给地名时导航页是空的，
// 这正是一开始"点了导航但终点没带出来"的原因。
func TestBuildAirportNav_CarriesDestinationCoordinates(t *testing.T) {
	nav := BuildAirportNav("pvg", nil)
	if nav == nil {
		t.Fatal("大小写不敏感地查表失败")
	}

	for _, want := range []string{"dlat=", "dlon=", "dname=", "dev=0", "t=0", "sourceApplication="} {
		if !strings.Contains(nav.App, want) {
			t.Fatalf("App 链接缺少参数 %s：%s", want, nav.App)
		}
	}
	// 目的地中文名要经过 URL 编码
	if !strings.Contains(nav.App, "%E4%B8%8A%E6%B5%B7") { // 上海
		t.Fatalf("目的地应经过 URL 编码: %s", nav.App)
	}

	// 网页版的 to 是 lon,lat[,name]，经度在前
	toLng, toLat := coordPair(t, queryParam(t, nav.Web, "to"))
	if toLng < 121.7 || toLng > 121.9 {
		t.Fatalf("to 的第一个分量应当是经度（浦东约 121.8），实际 %v：%s", toLng, nav.Web)
	}
	if toLat < 31.0 || toLat > 31.3 {
		t.Fatalf("to 的第二个分量应当是纬度（浦东约 31.1），实际 %v：%s", toLat, nav.Web)
	}

	// 没有定位时不带起点，交给高德自己决定
	if strings.Contains(nav.App, "slat=") || strings.Contains(nav.Web, "from=") {
		t.Fatalf("没有定位时不应带起点：%s / %s", nav.App, nav.Web)
	}
}

// 有定位时两条链接都要显式带上起点。
//
// 网页版的 from 留空只在移动端自动补当前位置，PC 端不会补 ——
// 不带起点的话，在电脑上点导航依然是一个空框。
func TestBuildAirportNav_CarriesOriginWhenLocated(t *testing.T) {
	origin := &domain.Coordinate{Lat: 31.2304, Lng: 121.4737} // 上海市中心

	nav := BuildAirportNav("PVG", origin)
	if nav == nil {
		t.Fatal("查表失败")
	}

	slat := queryParam(t, nav.App, "slat")
	slon := queryParam(t, nav.App, "slon")
	originLat, err := strconv.ParseFloat(slat, 64)
	if err != nil {
		t.Fatalf("slat 不是数字: %q", slat)
	}
	originLng, err := strconv.ParseFloat(slon, 64)
	if err != nil {
		t.Fatalf("slon 不是数字: %q", slon)
	}
	// 起点转成 GCJ-02 后仍应落在市中心附近，偏移只有几百米
	if math.Abs(originLng-121.4737) > 0.01 || math.Abs(originLat-31.2304) > 0.01 {
		t.Fatalf("起点坐标偏移过大：%v,%v", originLng, originLat)
	}

	fromLng, fromLat := coordPair(t, queryParam(t, nav.Web, "from"))
	if fromLng < 121.4 || fromLng > 121.5 {
		t.Fatalf("from 的第一个分量应当是经度，实际 %v", fromLng)
	}
	if fromLat < 31.2 || fromLat > 31.3 {
		t.Fatalf("from 的第二个分量应当是纬度，实际 %v", fromLat)
	}

	// 起点与终点要同时存在，且是不同位置
	if !strings.Contains(nav.App, "dlat=") || !strings.Contains(nav.App, "slat=") {
		t.Fatalf("App 链接应同时带起点与终点：%s", nav.App)
	}
	if strings.Contains(queryParam(t, nav.Web, "to"), queryParam(t, nav.Web, "from")) {
		t.Fatalf("起点与终点不应相同：%s", nav.Web)
	}
}

// 定位要一路传到导航链接里：Input.Progress.Location -> BuildAirportNav。
func TestEvaluate_NavCarriesReportedLocation(t *testing.T) {
	client := &fakeClient{content: `{
		"stage": "en_route",
		"risk": "yellow",
		"alert": null,
		"cards": [],
		"actions": [{"id": "leave_now", "title": "尽快出发", "detail": "预计 52 分钟到机场", "nav": "airport"}],
		"reasons": []
	}`}

	got := NewAgent(client).Evaluate(context.Background(), Input{
		State:       readyState(),
		AirportIATA: "PVG",
		Progress:    domain.JourneyProgress{Location: &domain.Coordinate{Lat: 31.2304, Lng: 121.4737}},
	})

	if len(got.Actions) != 1 || got.Actions[0].Nav == nil {
		t.Fatalf("行动与导航链接都该保留：%+v", got.Actions)
	}
	nav := got.Actions[0].Nav
	if !strings.Contains(nav.App, "slat=") || !strings.Contains(nav.Web, "from=") {
		t.Fatalf("导航链接应带上旅客定位：%+v", nav)
	}
}

// 导航不跟着风险降级一起消失。
//
// 这个用例同时钉住两件事：带判断的"立即出发"在关键数据缺失时必须被撤掉，
// 而"打开地图"这个工具动作必须补回来 —— 否则数据源不全时旅客连地图都打不开，
// 而那恰恰是最需要自己看路况的时候。
func TestEnsureAirportNav_SurvivesRiskDowngrade(t *testing.T) {
	client := &fakeClient{content: `{
		"stage": "en_route",
		"risk": "green",
		"alert": null,
		"cards": [],
		"actions": [{"id": "leave_now", "title": "尽快出发", "detail": "预计 52 分钟到机场", "nav": "airport"}],
		"reasons": []
	}`}

	state := readyState()
	state.ETAMin = nil // 路程时间缺失，判断不了来不来得及

	got := NewAgent(client).Evaluate(context.Background(), Input{
		State:       state,
		AirportIATA: "PVG",
		Progress:    domain.JourneyProgress{Location: &domain.Coordinate{Lat: 31.2304, Lng: 121.4737}},
	})

	if got.Risk != domain.RiskUnknown && got.Risk != domain.RiskYellow {
		t.Fatalf("关键数据缺失时必须降级（unknown 或 yellow），实际 %s", got.Risk)
	}

	// 降级后至少有一条带导航链接的行动（可能是模型给的 leave_now，也可能是我们补的 navigate_airport）
	var navAction *domain.Action
	for i := range got.Actions {
		if got.Actions[i].Nav != nil {
			navAction = &got.Actions[i]
			break
		}
	}
	if navAction == nil {
		t.Fatalf("降级后必须至少有一条带导航链接的行动：%+v", got.Actions)
	}
	if !strings.Contains(navAction.Nav.App, "dlat=") || !strings.Contains(navAction.Nav.App, "slat=") {
		t.Fatalf("App 链接应同时带起点与终点：%s", navAction.Nav.App)
	}
	if !strings.Contains(navAction.Nav.Web, "to=") || !strings.Contains(navAction.Nav.Web, "from=") {
		t.Fatalf("Web 链接应同时带起点与终点：%s", navAction.Nav.Web)
	}
}

// 没有定位就不给导航：PC 端不会自动补起点，给了就是两个空框。
func TestEnsureAirportNav_NeedsLocation(t *testing.T) {
	got := NewAgent(nil).Evaluate(context.Background(), Input{
		State:       readyState(),
		AirportIATA: "PVG",
	})

	if len(got.Actions) != 0 {
		t.Fatalf("没有定位时不该给导航，实际 %d 条", len(got.Actions))
	}
}

// 人已经进了机场（或已经起飞）就不用再"导航到机场"。
func TestEnsureAirportNav_SkippedWhenAlreadyAtAirport(t *testing.T) {
	in := locatedInput("PVG")

	for _, stage := range []string{
		domain.StageAtAirport,
		domain.StageCheckIn,
		domain.StageSecurity,
		domain.StageWaiting,
		domain.StageBoarding,
		domain.StageDeparted,
		domain.StageDisrupted,
	} {
		result := domain.NewAdvice()
		result.Stage = stage

		ensureAirportNav(&result, in)

		if len(result.Actions) != 0 {
			t.Fatalf("阶段 %s 不该再给导航，实际 %d 条", stage, len(result.Actions))
		}
	}
}

// 模型已经给过带导航的行动时，不重复添加。
func TestEnsureAirportNav_NoDuplicate(t *testing.T) {
	in := locatedInput("PVG")

	result := domain.NewAdvice()
	result.Stage = domain.StageEnRoute
	result.Actions = []domain.Action{
		{ID: "leave_now", Title: "尽快出发", Detail: "现在走", Nav: BuildAirportNav("PVG", in.Progress.Location)},
	}

	ensureAirportNav(&result, in)

	if len(result.Actions) != 1 {
		t.Fatalf("不该重复添加，实际 %d 条", len(result.Actions))
	}
}

// 三字码不在对照表里时不猜目的地。
func TestEnsureAirportNav_SkippedForUnknownAirport(t *testing.T) {
	in := locatedInput("ZZZ")

	result := domain.NewAdvice()
	result.Stage = domain.StageEnRoute

	ensureAirportNav(&result, in)

	if len(result.Actions) != 0 {
		t.Fatalf("查不到机场时不该给导航，实际 %d 条", len(result.Actions))
	}
}

func locatedInput(iata string) Input {
	return Input{
		AirportIATA: iata,
		Progress:    domain.JourneyProgress{Location: &domain.Coordinate{Lat: 31.2304, Lng: 121.4737}},
	}
}

func findAction(t *testing.T, actions []domain.Action, id string) domain.Action {
	t.Helper()
	for _, a := range actions {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("没有找到 id=%s 的行动：%+v", id, actions)
	return domain.Action{}
}

// 坐标转换的边界：境外原样返回，境内偏移量在合理范围。
//
// 不比对某个"标准答案"——GCJ-02 的偏移公式本身是拟合出来的。
// 要保证的是量级和方向没错：漏转和瞎转都会被量级检查抓住。
func TestWGS84ToGCJ02(t *testing.T) {
	if lng, lat := geo.WGS84ToGCJ02(-0.1276, 51.5074); lng != -0.1276 || lat != 51.5074 {
		t.Fatalf("境外坐标不应偏移，实际 %v,%v", lng, lat)
	}

	lng, lat := geo.WGS84ToGCJ02(121.4737, 31.2304) // 上海
	dlng, dlat := lng-121.4737, lat-31.2304
	if dlng == 0 && dlat == 0 {
		t.Fatal("境内坐标必须发生偏移，否则说明忘了转")
	}
	if math.Abs(dlng) > 0.01 || math.Abs(dlat) > 0.01 {
		t.Fatalf("偏移量过大（超过约 1 公里）：%v,%v", dlng, dlat)
	}
}

// queryParam 取出链接里的一个查询参数。u.Query() 会做 URL 解码，
// 这里只关心坐标部分，解码不影响断言。
func queryParam(t *testing.T, raw, key string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("解析链接失败 %s: %v", raw, err)
	}
	value := u.Query().Get(key)
	if value == "" {
		t.Fatalf("链接里没有参数 %s：%s", key, raw)
	}
	return value
}

// coordPair 解析 "经度,纬度" 形式的前两个分量。
func coordPair(t *testing.T, value string) (float64, float64) {
	t.Helper()
	parts := strings.SplitN(value, ",", 3)
	if len(parts) < 2 {
		t.Fatalf("坐标格式不对: %q", value)
	}
	lng, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		t.Fatalf("经度解析失败 %q: %v", parts[0], err)
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		t.Fatalf("纬度解析失败 %q: %v", parts[1], err)
	}
	return lng, lat
}

func hasReason(reasons []string, keyword string) bool {
	for _, r := range reasons {
		if strings.Contains(r, keyword) {
			return true
		}
	}
	return false
}

// TestUserPromptCarriesLocationFlag 锁住一件事：阶段判断依赖"有没有定位"，
// 但坐标本身不下发给模型（隐私最小化）。
func TestUserPromptCarriesLocationFlag(t *testing.T) {
	state := readyState()

	t.Run("有定位时带 locationProvided", func(t *testing.T) {
		prompt, err := userPrompt(Input{
			State:    state,
			Progress: domain.JourneyProgress{Location: &domain.Coordinate{Lat: 22.5431, Lng: 114.0579}},
		})
		if err != nil {
			t.Fatalf("构造 prompt 失败: %v", err)
		}
		if !strings.Contains(prompt, `"locationProvided": true`) {
			t.Fatalf("有定位时应下发 locationProvided：\n%s", prompt)
		}
		if strings.Contains(prompt, "114.0579") || strings.Contains(prompt, "22.5431") {
			t.Fatalf("不应把精确坐标下发给模型：\n%s", prompt)
		}
	})

	t.Run("无定位时不带该字段", func(t *testing.T) {
		prompt, err := userPrompt(Input{State: state})
		if err != nil {
			t.Fatalf("构造 prompt 失败: %v", err)
		}
		if strings.Contains(prompt, "locationProvided") {
			t.Fatalf("没有定位时不应出现 locationProvided：\n%s", prompt)
		}
	})
}

// TestUserPromptCarriesBaggage 锁住"托运行李会影响建议"这条输入契约：
// 有无托运必须明确下发给模型，否则建议里永远不会有托运环节。
func TestUserPromptCarriesBaggage(t *testing.T) {
	withBaggage, err := userPrompt(Input{State: readyState(), HasBaggage: true})
	if err != nil {
		t.Fatalf("构造 prompt 失败: %v", err)
	}
	if !strings.Contains(withBaggage, `"hasBaggage": true`) {
		t.Fatalf("带托运行李时应下发 hasBaggage=true：\n%s", withBaggage)
	}

	withoutBaggage, err := userPrompt(Input{State: readyState()})
	if err != nil {
		t.Fatalf("构造 prompt 失败: %v", err)
	}
	if !strings.Contains(withoutBaggage, `"hasBaggage": false`) {
		t.Fatalf("不带行李时也应明确下发 hasBaggage=false：\n%s", withoutBaggage)
	}
}

// TestUserPromptCarriesTimeFacts 锁住"时间事实必须下发"：
// 模型看不到当前时间和缓冲量，就会出现"起飞还有 8 小时却提示时间非常紧张"。
func TestUserPromptCarriesTimeFacts(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 36, 0, 0, time.FixedZone("CST", 8*3600))
	state := readyState()

	prompt, err := userPrompt(Input{
		State:    state,
		Progress: domain.JourneyProgress{Location: &domain.Coordinate{Lat: 31.23, Lng: 121.47}},
		Now:      now,
	})
	if err != nil {
		t.Fatalf("构造 prompt 失败: %v", err)
	}

	for _, want := range []string{
		`"serverNow": "2026-09-13T12:36:00+08:00"`,
		`"minutesUntilGateClose"`,
		`"bufferMin"`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt 应包含时间事实 %s：\n%s", want, prompt)
		}
	}
}

func marginState() domain.State {
	state := domain.NewState()
	state.FlightStatus = domain.FlightStatusScheduled
	state.Timeline = []domain.TimelineNode{
		{Label: "登机口关闭", Time: "2026-09-22T18:35:00+08:00"},
		{Label: "起飞", Time: "2026-09-22T18:50:00+08:00"},
	}
	return state
}

// TestEvaluate_ComfortableMarginUpgradesUnknownToYellow：
// 没有定位、算不出缓冲，但距登机口关闭还有 8 小时多 —— 不该是 unknown（那是"什么都判断不了"），
// 也不该是 green（缺路程时间），按契约允许的下限给 yellow。
func TestEvaluate_ComfortableMarginUpgradesUnknownToYellow(t *testing.T) {
	client := &fakeClient{content: `{
		"stage": "unknown",
		"risk": "unknown",
		"alert": null,
		"cards": [],
		"actions": [],
		"reasons": []
	}`}
	now := time.Date(2026, 9, 22, 10, 9, 0, 0, time.FixedZone("CST", 8*3600))

	got := NewAgent(client).Evaluate(context.Background(), Input{State: marginState(), Now: now})

	if got.Risk != domain.RiskYellow {
		t.Fatalf("时间充裕但缺路程时间时应为 yellow，实际 %s", got.Risk)
	}
	if !hasReason(got.Reasons, "时间看起来充裕") {
		t.Fatalf("reasons 应说明为什么按 yellow 处理: %v", got.Reasons)
	}
}

// TestEvaluate_TightMarginKeepsUnknownWithoutETA：时间并不充裕时不要硬升档，
// 该 unknown 就 unknown。
func TestEvaluate_TightMarginKeepsUnknownWithoutETA(t *testing.T) {
	client := &fakeClient{content: `{
		"stage": "unknown",
		"risk": "unknown",
		"alert": null,
		"cards": [],
		"actions": [],
		"reasons": []
	}`}
	now := time.Date(2026, 9, 22, 18, 0, 0, 0, time.FixedZone("CST", 8*3600))

	got := NewAgent(client).Evaluate(context.Background(), Input{State: marginState(), Now: now})

	if got.Risk != domain.RiskUnknown {
		t.Fatalf("只剩 35 分钟又没有路程时间时应保持 unknown，实际 %s", got.Risk)
	}
}
