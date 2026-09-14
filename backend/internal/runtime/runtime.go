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
// 分析要调用大模型和多个数据技能，所以给得比普通请求宽。
const analyzeTimeout = 90 * time.Second

// Runtime 编排 DataAgent 与 AdviceAgent，负责快照的保存与推送。
//
// 它是唯一知道"先取数、再决策"顺序的地方：
// Agent 不碰 store，不碰 SSE，也不碰 HTTP。
type Runtime struct {
	store  *store.MemoryStore
	data   *dataagent.Agent
	advice *adviceagent.Agent
	hub    *SSEHub
	// busy 标记正在分析中的行程。
	// 模型调用慢的时候，周期刷新会和上一轮撞上，用它能避免任务越积越多。
	busy sync.Map
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
// 同一行程同一时刻只跑一次：周期刷新撞上正在进行的分析时直接返回当前快照，
// 不排队等待。模型调用可能几十秒，排队会让任务越积越多。
//
// 分析失败不返回 error，而是产出一份 status = failed 的快照：
// 前端需要的是一个能渲染的结果，而不是一个 500。
func (r *Runtime) Recalculate(ctx context.Context, id string) (domain.JourneySnapshot, error) {
	if _, running := r.busy.LoadOrStore(id, struct{}{}); running {
		return r.store.GetSnapshot(id)
	}
	defer r.busy.Delete(id)

	journey, err := r.store.GetJourney(id)
	if err != nil {
		return domain.JourneySnapshot{}, err
	}
	progress := r.store.GetProgress(id)
	previous, hasPrevious := r.previousSnapshot(id)

	input := dataagent.Input{Journey: journey, Progress: progress}
	if hasPrevious {
		input.Previous = &previous.State
	}

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

	return r.finish(id, domain.JourneySnapshot{
		Status:  domain.StatusReady,
		Journey: journey,
		State:   r.stamp(result.State),
		Advice:  r.decide(ctx, result, progress, journey, previous, hasPrevious),
	}), nil
}

// decide 决定这一轮用新生成的建议，还是复用上一版。
//
// 状态没有实质变化时直接复用：模型调用又慢又费额度，
// 15 秒一次的周期刷新没必要每次都重算建议。
// 判据是除时间戳外 State 完全一致。
func (r *Runtime) decide(
	ctx context.Context,
	result dataagent.Result,
	progress domain.JourneyProgress,
	journey domain.Journey,
	previous domain.JourneySnapshot,
	hasPrevious bool,
) domain.Advice {
	if hasPrevious && previous.Status == domain.StatusReady && sameState(result.State, previous.State) {
		return previous.Advice
	}

	return r.advice.Evaluate(ctx, adviceagent.Input{
		State:       result.State,
		Progress:    progress,
		Issues:      result.Issues,
		AirportIATA: departureAirport(journey),
	})
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
