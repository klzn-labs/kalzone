package queues

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/stretchr/testify/require"
)

func adaptiveRow(a *entities.Adaptive, effective float64) Row {
	q := entities.Queue{Settings: entities.Settings{RatePerSecond: 10, Adaptive: true, MinRatePerSecond: 0.5}}
	return Row{Queue: q, Fresh: true, Stats: entities.Stats{EffectiveRate: effective, Adaptive: a}}
}

func Test_AFixedQueueShowsItsRateAndNoBadge(t *testing.T) {
	r := Row{Queue: entities.Queue{Settings: entities.Settings{RatePerSecond: 5}}, Fresh: true, Stats: entities.Stats{EffectiveRate: 5}}

	v := rateView(r)

	require.Equal(t, RateView{Now: "5 / s", Hint: "of 5 / s, fixed"}, v)
}

func Test_AStaleQueueShowsNoRate(t *testing.T) {
	r := adaptiveRow(nil, 0)
	r.Fresh = false

	v := rateView(r)

	require.Equal(t, "—", v.Now)
	require.Equal(t, "Adaptive", v.Badge)
	require.False(t, v.Reading)
}

func Test_ASlowedQueueShowsTheReadingOverTarget(t *testing.T) {
	v := rateView(adaptiveRow(&entities.Adaptive{Factor: 0.73, DatabaseCPU: 54.04, TargetCPU: 50}, 7.435))

	require.Equal(t, "7.4 / s", v.Now)
	require.Equal(t, "of 10 / s, adaptive", v.Hint)
	require.Equal(t, "Adaptive · slowing down", v.Badge)
	require.Equal(t, "badge-warn", v.BadgeClass)
	require.Equal(t, "DB CPU 54% of a core, target 50% · never below 0.5 / s", v.Line)
	require.True(t, v.Over)
	require.Equal(t, 54.0, v.CPUWidth)
}

func Test_ACalmQueueRunsAtFullSpeed(t *testing.T) {
	v := rateView(adaptiveRow(&entities.Adaptive{Factor: 1, DatabaseCPU: 8, TargetCPU: 50}, 10))

	require.Equal(t, "Adaptive · full speed", v.Badge)
	require.False(t, v.Over)
	require.Equal(t, 8.0, v.CPUWidth)
}

func Test_TheMeterStopsAtTwiceTheTarget(t *testing.T) {
	v := rateView(adaptiveRow(&entities.Adaptive{Factor: 0.1, DatabaseCPU: 250, TargetCPU: 50}, 1.4))

	require.Equal(t, 100.0, v.CPUWidth)
}

func Test_AQueueInFailSafeSaysItHasNoReading(t *testing.T) {
	v := rateView(adaptiveRow(&entities.Adaptive{Factor: 0.75, FailSafe: true, TargetCPU: 50}, 7.6))

	require.Equal(t, "Adaptive · no CPU reading", v.Badge)
	require.Equal(t, "Database CPU unknown; climbing back to 10 / s.", v.Line)
	require.False(t, v.Reading)
}
