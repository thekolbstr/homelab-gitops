package alerts

import (
	"sync"
	"time"
)

type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusDown      Status = "down"
	StatusRecovered Status = "recovered"
)

// Event represents a single health transition for a pod or deployment.
type Event struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Kind      string    `json:"kind"` // "pod" | "deployment"
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"`
	Cluster   string    `json:"cluster"`
	Status    Status    `json:"status"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
}

// ObjectState tracks the last known status of a watched object so the watcher
// only emits an Event when the status actually changes.
type ObjectState struct {
	Status       Status
	LastChanged  time.Time
	RestartCount int32
}

type Store struct {
	mu     sync.RWMutex
	events []Event
	states map[string]*ObjectState // key: namespace/kind/name
	max    int
}

func NewStore() *Store {
	return &Store{
		events: make([]Event, 0, 512),
		states: make(map[string]*ObjectState),
		max:    500,
	}
}

func key(namespace, kind, name string) string {
	return namespace + "/" + kind + "/" + name
}

func (s *Store) GetState(namespace, kind, name string) (*ObjectState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.states[key(namespace, kind, name)]
	return st, ok
}

func (s *Store) SetState(namespace, kind, name string, st *ObjectState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[key(namespace, kind, name)] = st
}

func (s *Store) Add(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	if len(s.events) > s.max {
		s.events = s.events[len(s.events)-s.max:]
	}
}

func (s *Store) Recent(limit int) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.events) {
		limit = len(s.events)
	}
	out := make([]Event, limit)
	copy(out, s.events[len(s.events)-limit:])
	return out
}

func (s *Store) Snapshot() map[string]*ObjectState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*ObjectState, len(s.states))
	for k, v := range s.states {
		out[k] = v
	}
	return out
}
