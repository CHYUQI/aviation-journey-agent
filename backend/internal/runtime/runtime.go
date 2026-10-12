package runtime

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	adviceagent "aviation-journey-agent/backend/internal/agent/advice"
	dataagent "aviation-journey-agent/backend/internal/agent/data"
	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/store"
)

// analyzeTimeout 是单次分析的超时上限。
//
// 分析要跑浏览器抓页面、调大模型抽字段、再调一次模型做决策，
// 数据源还可能逐个失败重试。90 秒实测会被耗光，导致后面的模型调用
// 直接 context deadline exceeded —— 那样连兜底建议都给不出来。
const analyzeTimeout = 180 * time.Second

// Runtime 编排 DataAgent 与 AdviceAgent，负责快照的保存与推送。
//
// 它是唯一知道"先取数、再决策"顺序的地方：
// Agent 不碰 store，不碰 SSE，也不碰 HTTP。
type Runtime struct {
	store  *store.MemoryStore
	data   *dataagent.Agent
	advice *adviceagent.Agent
	hub    *SSEHub
	// recalcStates 是每个行程的重算状态，保证同一行程的重算串行。
	// 模型调用慢的时候，周期刷新会和上一轮撞上，用它能避免任务越积越多。
	recalcStates sync.Map // id -> *recalcState
	// adviceInputs 记录上一版建议是用什么输入算出来的（见 sameAdviceInputs）。
	adviceInputs sync.Map // id -> adviceInputs
}

// recalcState 是单个行程的重算状态。
//
// running 表示当前是否有一轮分析在跑；dirty 表示跑的这一轮开始之后
// 又来了新的重算请求 —— 这些请求不能丢，必须在当前轮结束后补跑一次，
// 否则旅客上报的位置或阶段会停在旧快照里。
type recalcState struct {
	mu      sync.Mutex
	running bool
	dirty   bool
}

func (r *Runtime) recalcStateFor(id string) *recalcState {
	if v, ok := r.recalcStates.Load(id); ok {
		return v.(*recalcState)
	}
	created := &recalcState{}
	actual, _ := r.recalcStates.LoadOrStore(id, created)
	return actual.(*recalcState)
}

func New(s *store.MemoryStore, d *dataagent.Agent, a *adviceagent.Agent, hub *SSEHub) *Runtime {
	return &Runtime{store: s, data: d, advice: a, hub: hub}
}

// CreateJourney 保存行程并异步触发首次分析。
//
// 它立刻返回：分析涉及大模型，耗时可能十几秒，不能阻塞 HTTP 请求。
// 调用方返回 202 后，客户端通过 SSE 或 /state 获取结果。
func (r *Runtime) CreateJourney(journey domain.Journey) error {
	r.store.SaveJourney(journey)
	r.store.SaveProgress(journey.ID, domain.JourneyProgress{})
	r.publishProcessing(journey)
	r.analyzeAsync(journey.ID)
	return nil
}

// ApplyLocation 处理定位上报或手动确认阶段。
//
// 规则：上报坐标时清除此前手动确认的阶段，以定位为准。
// 调用方需先用 UpdateLocationRequest.Validate 校验请求。
func (r *Runtime) ApplyLocation(id string, req domain.UpdateLocationRequest) error {
	journey, err := r.store.GetJourney(id)
	if err != nil {
		return err
	}

	progress := r.store.GetProgress(id)
	if req.HasCoordinate() {
		progress.Location = &domain.Coordinate{Lat: *req.Lat, Lng: *req.Lng}
		progress.ManualStage = ""
	}
	if req.Stage != nil {
		progress.ManualStage = *req.Stage
	}
	r.store.SaveProgress(id, progress)

	r.publishProcessing(journey)
	r.analyzeAsync(id)
	return nil
}

// Recalculate 跑一次完整分析。
//
// 同一行程同一时刻只跑一轮：撞上正在进行的分析时不排队，而是记一个 dirty 标记，
// 由正在跑的那一轮结束后补跑一次 —— 合并连续上报、又不会把任何一次上报丢掉。
//
// 分析失败不返回 error，而是产出一份 status = failed 的快照：
// 前端需要的是一个能渲染的结果，而不是一个 500。
func (r *Runtime) Recalculate(ctx context.Context, id string) (domain.JourneySnapshot, error) {
	state := r.recalcStateFor(id)

	state.mu.Lock()
	if state.running {
		state.dirty = true
		state.mu.Unlock()
		return r.store.GetSnapshot(id)
	}
	state.running = true
	state.mu.Unlock()

	var (
		snapshot domain.JourneySnapshot
		err      error
	)
	for {
		snapshot, err = r.recalculateOnce(ctx, id)

		state.mu.Lock()
		// 只有在"没有新请求"或"这一轮已经超时"时才收工
		if !state.dirty || ctx.Err() != nil {
			state.dirty = false
			state.running = false
			state.mu.Unlock()
			return snapshot, err
		}
		state.dirty = false
		state.mu.Unlock()
	}
}

// recalculateOnce 是一轮完整的"取数 + 决策 + 落库 + 推送"。
func (r *Runtime) recalculateOnce(ctx context.Context, id string) (domain.JourneySnapshot, error) {
	journey, err := r.store.GetJourney(id)
	if err != nil {
		return domain.JourneySnapshot{}, err
	}
	progress := r.store.GetProgress(id)
	previous, hasPrevious := r.previousSnapshot(id)

	input := dataagent.Input{Journey: journey, Progress: progress}

	result, buildErr := r.data.BuildState(ctx, input)
	if buildErr != nil {
		msg := fmt.Sprintf("行程分析失败：%v", buildErr)
		return r.finish(id, domain.JourneySnapshot{
			Status:  domain.StatusFailed,
			Journey: journey,
			State:   r.stamp(domain.NewState()),
			Advice:  domain.NewAdvice(),
			Error:   &msg,
		}), nil
	}

	// DataAgent 可能用 EOOB 航班号搜索接口补全了最近班次和航线。
	// 把它写回 store，保证快照里的 journey 也是实际分析使用的那一份。
	if result.Journey.ID != "" {
		journey = result.Journey
		r.store.SaveJourney(journey)
	}

	// 先盖服务端时间戳再交给 AdviceAgent。
	// 模型看到的 state.updatedAt 必须是真实值，否则它会在 reasons 里写
	// "updatedAt 为空、无法确认数据时效性" 这种对旅客没有意义的话。
	result.State = r.stamp(result.State)

	return r.finish(id, domain.JourneySnapshot{
		Status:  domain.StatusReady,
		Journey: journey,
		State:   result.State,
		Advice:  r.decide(ctx, result, progress, journey, previous, hasPrevious),
	}), nil
}

// adviceInputs 是上一版建议实际用到的输入（时间戳除外）。
//
// ManualStage 必须在里面：手动确认的阶段只影响 advice.stage，不影响 state，
// 只看 state 是否变化，会在旅客改了阶段之后继续复用旧建议。
type adviceInputs struct {
	state       domain.State
	manualStage string
	issues      []string
}

// sameAdviceInputs 判断两组输入是否等价（State 忽略时间戳）。
func sameAdviceInputs(a, b adviceInputs) bool {
	if a.manualStage != b.manualStage {
		return false
	}
	if !sameState(a.state, b.state) {
		return false
	}
	if len(a.issues) != len(b.issues) {
		return false
	}
	for i := range a.issues {
		if a.issues[i] != b.issues[i] {
			return false
		}
	}
	return true
}

// decide 决定这一轮用新生成的建议，还是复用上一版。
//
// 输入没有实质变化时直接复用：模型调用又慢又费额度，
// 15 秒一次的周期刷新没必要每次都重算建议。
func (r *Runtime) decide(
	ctx context.Context,
	result dataagent.Result,
	progress domain.JourneyProgress,
	journey domain.Journey,
	previous domain.JourneySnapshot,
	hasPrevious bool,
) domain.Advice {
	inputs := adviceInputs{state: result.State, manualStage: progress.ManualStage, issues: result.Issues}

	if hasPrevious && previous.Status == domain.StatusReady {
		if cached, ok := r.adviceInputs.Load(journey.ID); ok {
			if prev, ok := cached.(adviceInputs); ok && sameAdviceInputs(prev, inputs) {
				return previous.Advice
			}
		}
	}

	// 用快照时间戳作为"现在"：它与推给前端的 updatedAt 是同一个服务端时钟，
	// 也是模型判断"还来不来得及"的唯一依据。
	now := time.Now()
	if ts, err := time.Parse(time.RFC3339, result.State.UpdatedAt); err == nil {
		now = ts
	}

	next := r.advice.Evaluate(ctx, adviceagent.Input{
		State:       result.State,
		Progress:    progress,
		Issues:      result.Issues,
		AirportIATA: departureAirport(journey),
		HasBaggage:  journey.HasBaggage,
		Now:         now,
	})

	// 刷新时模型/数据源临时故障，本轮建议会变成空壳（risk=unknown、没有卡片也没有行动）。
	// 直接落库等于把上一轮算好的建议擦掉，界面上就是"卡片突然消失"。
	// 这种降级不清空内容：保留上一轮建议，只把新的问题并进 reasons。
	if hasPrevious && previous.Status == domain.StatusReady &&
		next.Risk == domain.RiskUnknown && !hasAdviceContent(next) && hasAdviceContent(previous.Advice) {
		merged := previous.Advice
		if next.Stage != domain.StageUnknown {
			merged.Stage = next.Stage
		}
		// 风险取更保守的一侧：不做安全结论(unknown)排在最后，
		// 免得用旧结论盖住新一轮的数据缺失。
		merged.Risk = moreConservativeRisk(previous.Advice.Risk, next.Risk)
		merged.Reasons = mergeReasons(previous.Advice.Reasons, next.Reasons)
		// 刻意不记 adviceInputs：这一轮是降级复用，留下记录会让下一轮
		// 直接复用旧建议、不再尝试调用模型。
		return merged
	}

	r.adviceInputs.Store(journey.ID, inputs)
	return next
}

// riskRank 给风险等级排序，越大越保守。
var riskRank = map[string]int{
	domain.RiskGreen:   0,
	domain.RiskYellow:  1,
	domain.RiskOrange:  2,
	domain.RiskRed:     3,
	domain.RiskUnknown: 4,
}

// moreConservativeRisk 返回两者中更保守的风险等级。
func moreConservativeRisk(a, b string) string {
	if riskRank[b] > riskRank[a] {
		return b
	}
	return a
}

// hasAdviceContent 判断建议里有没有面向旅客的内容（卡片或行动）。
func hasAdviceContent(a domain.Advice) bool {
	return len(a.Cards) > 0 || len(a.Actions) > 0
}

// mergeReasons 合并两组依据：新的在前，去重且保持出现顺序。
func mergeReasons(newer, older []string) []string {
	seen := make(map[string]bool, len(newer)+len(older))
	merged := make([]string, 0, len(newer)+len(older))
	for _, group := range [][]string{newer, older} {
		for _, reason := range group {
			if reason == "" || seen[reason] {
				continue
			}
			seen[reason] = true
			merged = append(merged, reason)
		}
	}
	return merged
}

func (r *Runtime) GetSnapshot(id string) (domain.JourneySnapshot, error) {
	return r.store.GetSnapshot(id)
}

// Subscribe 订阅某行程的快照变化，返回的 cancel 必须被调用。
func (r *Runtime) Subscribe(id string) (<-chan domain.JourneySnapshot, func()) {
	return r.hub.Subscribe(id)
}

// finish 保存快照并广播给订阅者。所有写快照的地方都走这里，保证不漏推送。
func (r *Runtime) finish(id string, snapshot domain.JourneySnapshot) domain.JourneySnapshot {
	r.store.SaveSnapshot(snapshot)
	r.hub.Publish(id, snapshot)
	return snapshot
}

// publishProcessing 立刻推一帧 processing 快照，让前端马上切到骨架屏。
func (r *Runtime) publishProcessing(journey domain.Journey) {
	advice := domain.NewAdvice()
	advice.Reasons = []string{"分析进行中，暂无建议"}

	r.finish(journey.ID, domain.JourneySnapshot{
		Status:  domain.StatusProcessing,
		Journey: journey,
		State:   r.stamp(domain.NewState()),
		Advice:  advice,
	})
}

// stamp 给状态盖上快照生成时间。时间由服务端决定，客户端不上报时间。
func (r *Runtime) stamp(state domain.State) domain.State {
	state.UpdatedAt = nowRFC3339()
	return state
}

func (r *Runtime) previousSnapshot(id string) (domain.JourneySnapshot, bool) {
	snapshot, err := r.store.GetSnapshot(id)
	return snapshot, err == nil
}

// analyzeAsync 在后台跑一次分析，不阻塞调用方。
//
// 刻意不使用请求的 context：HTTP 请求在 202 返回后就结束了，
// 复用它会让刚启动的分析立刻被取消。
func (r *Runtime) analyzeAsync(id string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), analyzeTimeout)
		defer cancel()
		_, _ = r.Recalculate(ctx, id)
	}()
}

// sameState 比较两份状态是否实质相同，忽略时间戳。
func sameState(a, b domain.State) bool {
	a.UpdatedAt = ""
	b.UpdatedAt = ""
	return reflect.DeepEqual(a, b)
}

func departureAirport(journey domain.Journey) string {
	if len(journey.Flights) == 0 {
		return ""
	}
	return journey.Flights[0].From
}

func nowRFC3339() string { return time.Now().Format(time.RFC3339) }
