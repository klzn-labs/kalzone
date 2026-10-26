package governor_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/governor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

type firstOrderDatabase struct {
	clock    *fakeClock
	governor func() float64
	baseline float64
	gain     float64
	tau      time.Duration
	cpu      float64
}

func (d *firstOrderDatabase) Read(context.Context) (governor.Sample, error) {
	d.clock.at = d.clock.at.Add(period)
	target := d.baseline + d.gain*d.governor()
	alpha := 1 - math.Exp(-period.Seconds()/d.tau.Seconds())
	d.cpu += alpha * (target - d.cpu)
	return governor.Sample{Value: d.cpu, At: d.clock.at}, nil
}

func Test_TheLoopSettlesNearTheSetpointWithoutOscillating(t *testing.T) {
	clock := &fakeClock{at: time.Unix(1_700_000_000, 0)}
	db := &firstOrderDatabase{clock: clock, baseline: 10, gain: 120, tau: 45 * time.Second, cpu: 10}
	g, err := governor.New(db, settings(), clock.now, prometheus.NewRegistry())
	require.NoError(t, err)
	db.governor = func() float64 { return g.Adaptive().Factor }

	ticks(g, 200)
	settled := lastSamples(g, db, 40)

	require.InDelta(t, 50, mean(settled), 2)
	require.Less(t, spread(settled), 2.0)
}

func lastSamples(g *governor.Governor, db *firstOrderDatabase, n int) []float64 {
	result := make([]float64, 0, n)
	for range n {
		g.Tick(context.Background())
		result = append(result, db.cpu)
	}
	return result
}

func mean(values []float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total / float64(len(values))
}

func spread(values []float64) float64 {
	lo, hi := values[0], values[0]
	for _, v := range values {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	return hi - lo
}
