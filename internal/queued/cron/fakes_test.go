package cron_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/cron"
	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
)

type waiter struct {
	at time.Time
	ch chan time.Time
}

type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []waiter
}

func newClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 10, 23, 12, 0, 30, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	c.waiters = append(c.waiters, waiter{at: c.now.Add(d), ch: ch})
	return ch
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	pending := []waiter{}
	for _, w := range c.waiters {
		if w.at.After(c.now) {
			pending = append(pending, w)
			continue
		}
		w.ch <- c.now
	}
	c.waiters = pending
}

func (c *fakeClock) waiting() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.waiters)
}

type call struct {
	header http.Header
	body   string
}

type target struct {
	mu       sync.Mutex
	calls    []call
	answers  []int
	release  chan struct{}
	blocking bool
}

func (t *target) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	status, blocking := t.record(call{header: r.Header.Clone(), body: string(body)})
	if blocking {
		<-t.release
	}
	w.WriteHeader(status)
}

func (t *target) record(c call) (int, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls = append(t.calls, c)
	if len(t.answers) == 0 {
		return http.StatusOK, t.blocking
	}
	status := t.answers[0]
	t.answers = t.answers[1:]
	return status, t.blocking
}

func (t *target) received() []call {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]call{}, t.calls...)
}

type world struct {
	clock     *fakeClock
	target    *target
	server    *httptest.Server
	outbox    *cron.Outbox
	scheduler *cron.Scheduler
}

var fastBackoff = model.Backoff{Kind: model.BackoffFixed, Initial: 10 * time.Millisecond, Max: 10 * time.Millisecond}

func newWorld(t *testing.T, answers ...int) *world {
	t.Helper()
	w := &world{clock: newClock(), target: &target{answers: answers, release: make(chan struct{})}, outbox: cron.NewOutbox(100)}
	w.server = httptest.NewServer(w.target)
	ctx, cancel := context.WithCancel(context.Background())
	w.scheduler = cron.New(ctx, cron.Options{Client: w.server.Client(), Clock: w.clock, Backoff: fastBackoff, Outbox: w.outbox})
	t.Cleanup(func() {
		cancel()
		w.scheduler.StopAll()
		w.server.Close()
	})
	return w
}

func (w *world) desired(schedule string, paused bool) queuedapi.Cron {
	return queuedapi.Cron{
		ID: "c1", Name: "nightly", Environment: "dev", Service: "worker", Schedule: schedule, Target: w.server.URL + "/hooks/nightly",
		Payload: []byte(`{"full":true}`), Timeout: "2s", Paused: paused, SigningKey: "signing-key",
	}
}
