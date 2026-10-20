package sync_test

import (
	"context"
	gosync "sync"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/observe/handlers/sync"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/stretchr/testify/require"
)

type syncer struct {
	mu      gosync.Mutex
	calls   int
	running int
	overlap bool
	release chan struct{}
}

func (s *syncer) SyncsOn(action string) bool { return action == "database.attached" }

func (s *syncer) Sync(context.Context) error {
	s.mu.Lock()
	s.calls++
	s.running++
	s.overlap = s.overlap || s.running > 1
	s.mu.Unlock()
	if s.release != nil {
		<-s.release
	}
	s.mu.Lock()
	s.running--
	s.mu.Unlock()
	return nil
}

func (s *syncer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func started(t *testing.T, s *syncer, every time.Duration) *sync.Worker {
	t.Helper()
	w := sync.NewWorker(s, every)
	w.Start()
	t.Cleanup(w.Stop)
	return w
}

func Test_Worker_SyncsOnceAtStart(t *testing.T) {
	s := &syncer{}
	started(t, s, time.Hour)

	require.Eventually(t, func() bool { return s.count() == 1 }, time.Second, 5*time.Millisecond)
}

func Test_Worker_CoalescesEventsArrivingDuringASync(t *testing.T) {
	s := &syncer{release: make(chan struct{})}
	w := started(t, s, time.Hour)
	require.Eventually(t, func() bool { return s.count() == 1 }, time.Second, 5*time.Millisecond)

	for range 5 {
		require.NoError(t, w.Handle(context.Background(), events.Event{Action: "database.attached"}))
	}
	close(s.release)

	require.Eventually(t, func() bool { return s.count() == 2 }, time.Second, 5*time.Millisecond)
	require.Never(t, func() bool { return s.count() > 2 }, 100*time.Millisecond, 5*time.Millisecond)
	require.False(t, s.overlap)
}

func Test_Worker_IgnoresUnrelatedEvents(t *testing.T) {
	s := &syncer{}
	w := started(t, s, time.Hour)
	require.Eventually(t, func() bool { return s.count() == 1 }, time.Second, 5*time.Millisecond)

	require.NoError(t, w.Handle(context.Background(), events.Event{Action: "queue.created"}))

	require.Never(t, func() bool { return s.count() > 1 }, 100*time.Millisecond, 5*time.Millisecond)
}

func Test_Worker_ResyncsPeriodically(t *testing.T) {
	s := &syncer{}
	started(t, s, 20*time.Millisecond)

	require.Eventually(t, func() bool { return s.count() >= 3 }, time.Second, 5*time.Millisecond)
}
