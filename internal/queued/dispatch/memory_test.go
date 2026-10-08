package dispatch_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/adrianozp/platform/internal/queued/model"
)

type memory struct {
	mu        sync.Mutex
	waiting   int
	fetchedAt map[string]time.Time
	held      []time.Duration
}

func newMemory(n int) *memory {
	return &memory{waiting: n, fetchedAt: map[string]time.Time{}}
}

func (m *memory) Fetch(_ context.Context, q model.Queue, max int, _ time.Duration) ([]model.Delivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := []model.Delivery{}
	for ; max > 0 && m.waiting > 0; max-- {
		id := fmt.Sprintf("m%d", m.waiting)
		m.waiting--
		m.fetchedAt[id] = time.Now()
		result = append(result, model.Delivery{ID: id, QueueID: q.ID, Attempt: 1})
	}
	return result, nil
}

func (m *memory) Ack(_ context.Context, d model.Delivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.held = append(m.held, time.Since(m.fetchedAt[d.ID]))
	return nil
}

func (m *memory) longestHeld() (time.Duration, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	longest := time.Duration(0)
	for _, h := range m.held {
		longest = max(longest, h)
	}
	return longest, len(m.held)
}

func (m *memory) Ensure(context.Context, model.Queue) error                  { return nil }
func (m *memory) Remove(context.Context, model.Queue) error                  { return nil }
func (m *memory) MoveDue(context.Context, model.Queue, int) (int, error)     { return 0, nil }
func (m *memory) Retry(context.Context, model.Delivery, time.Duration) error { return nil }
func (m *memory) Replay(context.Context, model.Queue, string) error          { return nil }
func (m *memory) ReplayAll(context.Context, model.Queue) (int, error)        { return 0, nil }
func (m *memory) PurgeDead(context.Context, model.Queue) error               { return nil }
func (m *memory) Publish(context.Context, model.Queue, model.Outgoing) (string, error) {
	return "", nil
}
func (m *memory) Bury(context.Context, model.Queue, model.Delivery, model.DeadReason) error {
	return nil
}
func (m *memory) Dead(context.Context, model.Queue, int) ([]model.DeadLetter, error) {
	return nil, nil
}
func (m *memory) Stats(context.Context, model.Queue) (model.BackendStats, error) {
	return model.BackendStats{}, nil
}
