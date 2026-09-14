package store

import (
	"errors"
	"sync"

	"aviation-journey-agent/backend/internal/domain"
)

// ErrNotFound 表示行程不存在。
var ErrNotFound = errors.New("journey not found")

// MemoryStore 是进程内存储。课程项目不引入数据库，
// 演示时数据随进程生命周期存在。
type MemoryStore struct {
	mu        sync.RWMutex
	journeys  map[string]domain.Journey
	snapshots map[string]domain.JourneySnapshot
	progress  map[string]domain.JourneyProgress
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		journeys:  map[string]domain.Journey{},
		snapshots: map[string]domain.JourneySnapshot{},
		progress:  map[string]domain.JourneyProgress{},
	}
}

func (s *MemoryStore) SaveJourney(journey domain.Journey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.journeys[journey.ID] = journey
}

func (s *MemoryStore) GetJourney(id string) (domain.Journey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	journey, ok := s.journeys[id]
	if !ok {
		return domain.Journey{}, ErrNotFound
	}
	return journey, nil
}

func (s *MemoryStore) SaveSnapshot(snapshot domain.JourneySnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[snapshot.Journey.ID] = snapshot
}

func (s *MemoryStore) GetSnapshot(id string) (domain.JourneySnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot, ok := s.snapshots[id]
	if !ok {
		return domain.JourneySnapshot{}, ErrNotFound
	}
	return snapshot, nil
}

// SaveProgress 保存契约之外的内部状态（位置、手动确认的阶段）。
func (s *MemoryStore) SaveProgress(id string, progress domain.JourneyProgress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progress[id] = progress
}

// GetProgress 返回内部状态。行程还没上报过位置时返回零值，不报错。
func (s *MemoryStore) GetProgress(id string) domain.JourneyProgress {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.progress[id]
}

// ListJourneyIDs 返回当前所有行程 ID，供定时刷新使用。
func (s *MemoryStore) ListJourneyIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.journeys))
	for id := range s.journeys {
		ids = append(ids, id)
	}
	return ids
}
