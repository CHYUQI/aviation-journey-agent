package store

import (
	"aviation-journey-agent/backend/internal/domain"
	"errors"
	"sync"
)

type MemoryStore struct {
	mu        sync.RWMutex
	journeys  map[string]domain.Journey
	snapshots map[string]domain.JourneySnapshot
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{journeys: map[string]domain.Journey{}, snapshots: map[string]domain.JourneySnapshot{}}
}
func (s *MemoryStore) SaveJourney(journey domain.Journey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.journeys[journey.ID] = journey
}
func (s *MemoryStore) GetJourney(id string) (domain.Journey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.journeys[id]
	return v, ok
}
func (s *MemoryStore) SaveSnapshot(snapshot domain.JourneySnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[snapshot.Journey.ID] = snapshot
}
func (s *MemoryStore) GetSnapshot(id string) (domain.JourneySnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.snapshots[id]
	if !ok {
		return domain.JourneySnapshot{}, errors.New("journey snapshot not found")
	}
	return v, nil
}
