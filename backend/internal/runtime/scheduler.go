package runtime

import (
	"context"
	"time"
)

// StartPeriodicRefresh 定期重算活跃行程。SSE 只推送结果，不负责触发计算。
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
