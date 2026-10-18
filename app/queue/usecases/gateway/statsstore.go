package gateway

import (
	"sync"
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
)

type StatsStore struct {
	mu     sync.RWMutex
	latest map[string]entities.Stats
}

func NewStatsStore() *StatsStore {
	return &StatsStore{latest: map[string]entities.Stats{}}
}

func (s *StatsStore) Put(stats []entities.Stats, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range stats {
		st.ReportedAt = at
		s.latest[st.QueueID] = st
	}
}

func (s *StatsStore) Latest(queueID string) (entities.Stats, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.latest[queueID]
	return st, ok
}
