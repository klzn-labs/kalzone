package runner_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/runner"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
	"github.com/stretchr/testify/require"
)

type controlPlane struct {
	mu       sync.Mutex
	failures int
	calls    int
	done     []string
	reports  int
}

func (c *controlPlane) Desired(context.Context) (queuedapi.Desired, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.calls <= c.failures {
		return queuedapi.Desired{}, errors.New("connection refused")
	}
	return queuedapi.Desired{Queues: []queuedapi.Queue{{ID: "q1"}}, Commands: []queuedapi.Command{{ID: "c1"}}}, nil
}

func (c *controlPlane) Report(context.Context, []queuedapi.Stats) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reports++
	return nil
}

func (c *controlPlane) Done(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.done = append(c.done, id)
	return nil
}

func (c *controlPlane) snapshot() (int, []string, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls, append([]string{}, c.done...), c.reports
}

type applier struct {
	mu      sync.Mutex
	applied int
}

func (a *applier) Apply(_ context.Context, d queuedapi.Desired) ([]string, []error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applied++
	return []string{d.Commands[0].ID}, nil
}

func (a *applier) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.applied
}

type stats struct{}

func (stats) Collect(context.Context) []queuedapi.Stats { return []queuedapi.Stats{{QueueID: "q1"}} }

func Test_Run_SurvivesAnUnreachableControlPlaneAndReports(t *testing.T) {
	cp, a := &controlPlane{failures: 2}, &applier{}
	loop := runner.New(cp, a, stats{}, 10*time.Millisecond, 15*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go loop.Run(ctx)

	require.Eventually(t, func() bool { return a.count() >= 1 }, 2*time.Second, 5*time.Millisecond)
	calls, done, _ := cp.snapshot()
	require.GreaterOrEqual(t, calls, 3)
	require.Contains(t, done, "c1")
	require.Eventually(t, func() bool { _, _, reports := cp.snapshot(); return reports >= 1 }, 2*time.Second, 5*time.Millisecond)
}
