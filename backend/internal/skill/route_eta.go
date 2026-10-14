package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"aviation-journey-agent/backend/internal/geo"
)

// MaxRouteETAMinutes 是路程时间的可信上限（24 小时）。
//
// 超过它就说明输入本身有问题（定位坐标越界、把地球另一端的坐标传了进来），
// 或者这个行程根本不该用"驾车去机场"来建议。宁可没有数字，
// 也不能把 3 万分钟这种看着精确、实际荒谬的值推给模型和界面。
const MaxRouteETAMinutes = 24 * 60

// ValidRouteETAMinutes 判断一个路程时间是否可信。
// DataAgent 在写入 State.ETAMin 之前会再校验一次（技能可能来自别处）。
func ValidRouteETAMinutes(minutes int) bool {
	return minutes >= 1 && minutes <= MaxRouteETAMinutes
}

// RouteETASkill 计算当前位置到出发机场的预计驾车耗时。
//
// 数据源是高德 Web 服务"驾车路径规划"接口：
//
//	GET https://restapi.amap.com/v3/direction/driving?key=...&origin=lon,lat&destination=lon,lat
//
// 坐标必须是 GCJ-02，而浏览器定位与机场基准点都是 WGS-84，请求前统一用
// geo.WGS84ToGCJ02 转一次。返回的 route.paths[0].duration 是字符串秒数，
// 向上取整到分钟。
//
// 降级口径：
//   - 没有定位 → issue "缺少定位"
//   - 没配 key → issue "路径规划数据源尚未接入"
//   - 机场不在表里 → issue "机场 X 坐标未收录"
//   - 高德返回错误/网络失败 → issue "路径规划请求失败：..."，不阻断其他字段
type RouteETASkill struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewRouteETASkill 构造一个路程时间技能。
//
// apiKey 为空时技能仍可注册，但每次查询都会返回"尚未接入"issue——
// 这样本地开发不用配 key 也能编译启动，只是拿不到路程数字。
// baseURL 留空用高德官方端点；timeout<=0 默认 10 秒。
func NewRouteETASkill(apiKey, baseURL string, timeout time.Duration) *RouteETASkill {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://restapi.amap.com/v3/direction/driving"
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &RouteETASkill{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: strings.TrimSpace(baseURL),
		client:  &http.Client{Timeout: timeout},
	}
}

func (s *RouteETASkill) Name() string { return "route_eta" }

func (s *RouteETASkill) Supports(field string) bool { return field == FieldTravelETA }

func (s *RouteETASkill) Execute(ctx context.Context, query Query) (Result, error) {
	if query.Location == nil {
		return Result{Issues: []string{"缺少定位，无法计算路程时间"}}, nil
	}
	if s.apiKey == "" {
		return Result{Issues: []string{"路径规划数据源尚未接入"}}, nil
	}
	if len(query.Journey.Flights) == 0 {
		return Result{Issues: []string{"缺少航段，无法确定出发机场"}}, nil
	}

	airport, ok := geo.Airport(query.Journey.Flights[0].From)
	if !ok {
		return Result{Issues: []string{"机场 " + query.Journey.Flights[0].From + " 坐标未收录，无法计算路程时间"}}, nil
	}

	// WGS-84 → GCJ-02。注意入参顺序是 (lng, lat)。
	fromLng, fromLat := geo.WGS84ToGCJ02(query.Location.Lng, query.Location.Lat)
	toLng, toLat := geo.WGS84ToGCJ02(airport.Lng, airport.Lat)

	etaMin, err := s.drivingEta(ctx, fromLng, fromLat, toLng, toLat)
	if err != nil {
		return Result{Issues: []string{"路径规划请求失败：" + err.Error()}}, nil
	}

	return Result{
		Observations: []Observation{{
			Field:      FieldTravelETA,
			Value:      etaMin,
			Source:     "高德驾车路径规划",
			ObservedAt: time.Now().Format(time.RFC3339),
			Confidence: "high",
		}},
	}, nil
}

// drivingEta 调一次高德驾车接口，返回预计分钟数。
func (s *RouteETASkill) drivingEta(ctx context.Context, fromLng, fromLat, toLng, toLat float64) (int, error) {
	u := s.baseURL +
		"?key=" + url.QueryEscape(s.apiKey) +
		"&origin=" + coord6(fromLng) + "," + coord6(fromLat) +
		"&destination=" + coord6(toLng) + "," + coord6(toLat)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var parsed struct {
		Status string `json:"status"` // "1" 成功，"0" 失败
		Info   string `json:"info"`
		Route  struct {
			Paths []struct {
				Duration string `json:"duration"` // 字符串秒数
			} `json:"paths"`
		} `json:"route"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, fmt.Errorf("响应解析失败")
	}
	if parsed.Status != "1" || len(parsed.Route.Paths) == 0 {
		return 0, fmt.Errorf("高德返回错误：%s", parsed.Info)
	}

	sec, err := strconv.Atoi(strings.TrimSpace(parsed.Route.Paths[0].Duration))
	if err != nil || sec < 0 {
		return 0, fmt.Errorf("时长字段异常")
	}
	min := (sec + 59) / 60 // 向上取整
	if min < 1 {
		min = 1
	}
	return min, nil
}

// coord6 与 advice.coord 同款：定点表示、小数点后 6 位。
func coord6(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}
