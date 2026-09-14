package runtime

import (
	"sync"

	"aviation-journey-agent/backend/internal/domain"
)

// subscribeBuffer 是每个订阅者的缓冲深度。
//
// 慢客户端不会阻塞发布方：缓冲满了就丢弃这一帧，等下一次更新补上。
// 行程快照是"全量覆盖"语义，丢一帧不会导致状态错乱。
const subscribeBuffer = 4

// SSEHub 按行程 ID 广播快照。
//
// 它是"变化即推送"的实现基础：行程状态一变就 Publish，
// 所有订阅该行程的 SSE 连接立刻收到，不等下一个刷新周期。
type SSEHub struct {
	mu   sync.RWMutex
	subs map[string]map[chan domain.JourneySnapshot]struct{}
}

func NewSSEHub() *SSEHub {
	return &SSEHub{subs: map[string]map[chan domain.JourneySnapshot]struct{}{}}
}

// Subscribe 订阅某个行程的快照。返回的 cancel 必须被调用，否则会泄漏。
func (h *SSEHub) Subscribe(id string) (<-chan domain.JourneySnapshot, func()) {
	ch := make(chan domain.JourneySnapshot, subscribeBuffer)

	h.mu.Lock()
	if h.subs[id] == nil {
		h.subs[id] = map[chan domain.JourneySnapshot]struct{}{}
	}
	h.subs[id][ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			if subs, ok := h.subs[id]; ok {
				delete(subs, ch)
				if len(subs) == 0 {
					delete(h.subs, id)
				}
			}
			h.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancel
}

// Publish 把快照广播给该行程的所有订阅者。没有订阅者时直接返回。
func (h *SSEHub) Publish(id string, snapshot domain.JourneySnapshot) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.subs[id] {
		select {
		case ch <- snapshot:
		default:
			// 订阅者处理不过来，丢弃这一帧
		}
	}
}
