package runtime

import (
	"context"
	"time"

	"aviation-journey-agent/backend/internal/domain"
)

// refreshPerJourney 是单个行程的最小刷新间隔。
//
// 为什么需要它：一次分析要跑数据源 + 两次模型调用，实测 60~120s；
// 而 ticker 每 15s 就遍历一次全部行程 —— 等于对每个行程无限重算，
// 行程一多就并发撞模型配额与浏览器会话，还会把上一轮算好的建议冲掉。
const refreshPerJourney = 2 * time.Minute

// StartPeriodicRefresh 定期刷新"还值得刷新"的行程。
//
// 与之前的行为差异：
//  1. 每个行程按 refreshPerJourney 节流，不再每个 tick 都全量重算；
//  2. 已起飞/已取消，或出发日期已经过去的行程直接跳过（不会再变了）。
//
// 位置或阶段变化后的推送不依赖这里，走 SSEHub 立即推送。
func (r *Runtime) StartPeriodicRefresh(ctx context.Context, interval time.Duration, ids func() []string) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()

		lastRefresh := map[string]time.Time{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				for _, id := range ids() {
					if now.Sub(lastRefresh[id]) < refreshPerJourney {
						continue
					}
					lastRefresh[id] = now
					if !r.shouldRefresh(id, now) {
						continue
					}
					_, _ = r.Recalculate(ctx, id)
				}
			}
		}
	}()
}

// shouldRefresh 判断一个行程是否还需要继续刷新。
// 终态（已起飞、已取消）和日期已经过去的行程不再查，避免无意义地占模型与数据源。
func (r *Runtime) shouldRefresh(id string, now time.Time) bool {
	snapshot, err := r.store.GetSnapshot(id)
	if err != nil {
		return true
	}
	switch snapshot.State.FlightStatus {
	case domain.FlightStatusDeparted, domain.FlightStatusCancelled:
		return false
	}
	return !flightDatePassed(snapshot.Journey, now)
}

// flightDatePassed 判断出发日期是否已经过去。
//
// 只按服务器本地日期粗判：航班可能在别的时区，宁可多刷一天，
// 也不要因为算错时区把还在飞的行程停掉。
func flightDatePassed(journey domain.Journey, now time.Time) bool {
	if len(journey.Flights) == 0 {
		return false
	}
	date, err := time.Parse("2006-01-02", journey.Flights[0].Date)
	if err != nil {
		return false
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	return date.Before(today)
}
