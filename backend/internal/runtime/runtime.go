package runtime

import (
	"aviation-journey-agent/backend/internal/agent/advice"
	dataagent "aviation-journey-agent/backend/internal/agent/data"
	"aviation-journey-agent/backend/internal/domain"
	"aviation-journey-agent/backend/internal/store"
	"context"
	"fmt"
	"sync"
	"time"
)

type Runtime struct {
	store       *store.MemoryStore
	dataAgent   *dataagent.Agent
	adviceAgent *advice.Agent
	locks       sync.Map
}

func New(s *store.MemoryStore, d *dataagent.Agent, a *advice.Agent) *Runtime {
	return &Runtime{store: s, dataAgent: d, adviceAgent: a}
}
func (r *Runtime) CreateJourney(ctx context.Context, journey domain.Journey) (domain.JourneySnapshot, error) {
	r.store.SaveJourney(journey)
	return r.Recalculate(ctx, journey.ID)
}
func (r *Runtime) Recalculate(ctx context.Context, id string) (domain.JourneySnapshot, error) {
	journey, ok := r.store.GetJourney(id)
	if !ok {
		return domain.JourneySnapshot{}, fmt.Errorf("journey %s not found", id)
	}
	lock := r.journeyLock(id)
	lock.Lock()
	defer lock.Unlock()
	previous, err := r.store.GetSnapshot(id)
	if err != nil {
		previous = domain.JourneySnapshot{}
	}
	state, err := r.dataAgent.BuildState(ctx, journey, &previous.State)
	if err != nil {
		return domain.JourneySnapshot{}, err
	}
	result := domain.JourneySnapshot{Journey: journey, State: state, Advice: r.adviceAgent.Evaluate(state), GeneratedAt: time.Now().Format(time.RFC3339)}
	r.store.SaveSnapshot(result)
	return result, nil
}
func (r *Runtime) GetSnapshot(id string) (domain.JourneySnapshot, error) {
	return r.store.GetSnapshot(id)
}
func (r *Runtime) UpdateLocation(id string, location domain.Location) error {
	if _, ok := r.store.GetJourney(id); !ok {
		return fmt.Errorf("journey %s not found", id)
	}
	snapshot, err := r.store.GetSnapshot(id)
	if err != nil {
		return err
	}
	snapshot.State.Passenger.Location = &location
	snapshot.State.Travel.Location = &location
	r.store.SaveSnapshot(snapshot)
	return nil
}
func (r *Runtime) journeyLock(id string) *sync.Mutex {
	value, _ := r.locks.LoadOrStore(id, &sync.Mutex{})
	return value.(*sync.Mutex)
}
