package runtime

import (
	"context"
	"time"
)

// StartPeriodicRefresh 定期重算所有行程，用于刷新航班与机场状态。
//
// 注意：这只是"定期刷新"。位置或阶段变化后的推送走 SSEHub，
// 不等这里的周期。
func (r *Runtime) StartPeriodicRefresh(ctx context.Context, interval time.Duration, ids func() []string) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, id := range ids() {
					_, _ = r.Recalculate(ctx, id)
				}
			}
		}
	}()
}
