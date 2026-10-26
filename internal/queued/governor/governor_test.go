package governor_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/governor"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

const period = 15 * time.Second

type fakeClock struct{ at time.Time }

func (c *fakeClock) now() time.Time { return c.at }

type scriptedReader struct {
	clock *fakeClock
	cpu   float64
	err   error
	age   time.Duration
}

func (r *scriptedReader) Read(context.Context) (governor.Sample, error) {
	r.clock.at = r.clock.at.Add(period)
	return governor.Sample{Value: r.cpu, At: r.clock.at.Add(-r.age)}, r.err
}

func settings() governor.Settings {
	return governor.Settings{Setpoint: 50, Kp: 0.01, Ki: 0.001, Period: period, StaleAfter: 3, FilterSize: 1}
}

func newGovernor(t *testing.T, cpu float64) (*governor.Governor, *scriptedReader, *prometheus.Registry) {
	t.Helper()
	clock := &fakeClock{at: time.Unix(1_700_000_000, 0)}
	reader := &scriptedReader{clock: clock, cpu: cpu}
	registry := prometheus.NewRegistry()
	g, err := governor.New(reader, settings(), clock.now, registry)
	require.NoError(t, err)
	return g, reader, registry
}

func ticks(g *governor.Governor, n int) {
	for range n {
		g.Tick(context.Background())
	}
}

func Test_AnIdleDatabaseKeepsTheFullRateFromTheFirstTick(t *testing.T) {
	g, _, _ := newGovernor(t, 5)

	ticks(g, 1)

	require.Equal(t, 1.0, g.Adaptive().Factor)
	require.Equal(t, 5.0, g.Adaptive().Measured)
}

func Test_ABusyDatabaseLowersTheFactorWithinItsBounds(t *testing.T) {
	g, _, _ := newGovernor(t, 400)

	ticks(g, 40)

	require.Equal(t, 0.0, g.Adaptive().Factor)
}

func Test_FailSafeAfterStaleAfterBadReadsThenBackOnAGoodOne(t *testing.T) {
	g, reader, registry := newGovernor(t, 400)
	ticks(g, 40)
	reader.err = errors.New("prometheus is down")

	ticks(g, 2)
	held := g.Adaptive()
	ticks(g, 1)
	entered := g.Adaptive()
	ticks(g, 3)
	recovered := g.Adaptive()
	reader.err, reader.cpu = nil, 5
	ticks(g, 1)

	require.False(t, held.FailSafe)
	require.Equal(t, 0.0, held.Factor)
	require.True(t, entered.FailSafe)
	require.Equal(t, 0.25, entered.Factor)
	require.Equal(t, 1.0, recovered.Factor)
	require.False(t, g.Adaptive().FailSafe)
	require.Equal(t, 1.0, g.Adaptive().Factor)
	require.Equal(t, 1.0, transitions(t, registry, "enter"))
	require.Equal(t, 1.0, transitions(t, registry, "leave"))
}

func Test_OldReadsCountAsBadReads(t *testing.T) {
	g, reader, _ := newGovernor(t, 5)
	reader.age = 3 * period

	ticks(g, 3)

	require.True(t, g.Adaptive().FailSafe)
}

func Test_NotANumberCountsAsABadRead(t *testing.T) {
	g, _, _ := newGovernor(t, math.NaN())

	ticks(g, 3)

	require.True(t, g.Adaptive().FailSafe)
}

func transitions(t *testing.T, registry *prometheus.Registry, direction string) float64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() == "queued_adaptive_failsafe_transitions_total" {
			return counterWithLabel(f.GetMetric(), direction)
		}
	}
	return 0
}

func counterWithLabel(metrics []*dto.Metric, value string) float64 {
	for _, m := range metrics {
		if m.GetLabel()[0].GetValue() == value {
			return m.GetCounter().GetValue()
		}
	}
	return 0
}
