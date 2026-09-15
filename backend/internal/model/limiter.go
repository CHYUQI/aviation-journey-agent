package model

import "context"

// callSlots 限制同进程内并发的模型调用数，默认 1（串行）。
//
// 为什么默认串行：实测两个行程同时分析时，百炼兼容端点会在 45s 后才以
// "context deadline exceeded (Client.Timeout exceeded while awaiting headers)"
// 失败（复现 3/5）；同一时刻单独跑一条 modelcheck 只要 2~3s。
// 串行之后这种"并发打爆配额"的超时不再出现。
// 需要吞吐时用 MODEL_MAX_CONCURRENCY 调大（必须在启动阶段调用）。
var callSlots = make(chan struct{}, 1)

// SetMaxConcurrency 设置模型调用并发上限，n <= 0 表示不限制（内部取 64）。
func SetMaxConcurrency(n int) {
	if n <= 0 {
		n = 64
	}
	callSlots = make(chan struct{}, n)
}

// acquireCallSlot 抢一个调用额度；ctx 结束就放弃，避免超时后还堵在这里。
func acquireCallSlot(ctx context.Context) error {
	select {
	case callSlots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseCallSlot() {
	select {
	case <-callSlots:
	default:
	}
}
