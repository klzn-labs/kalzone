package entities_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/stretchr/testify/require"
)

func samples(values ...float64) []entities.Sample {
	start := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	result := []entities.Sample{}
	for i, v := range values {
		result = append(result, entities.Sample{At: start.Add(time.Duration(i) * 5 * time.Minute), Value: v})
	}
	return result
}

func Test_AReadingKnowsItsPeakAndHowLongItWasOver(t *testing.T) {
	r := entities.ReadingOf(entities.MetricDBCPU, 70, samples(40, 72, 78, 71, 46), 5*time.Minute)

	require.Equal(t, 46.0, r.Now)
	require.Equal(t, 78.0, r.Peak)
	require.Equal(t, time.Date(2026, 10, 21, 0, 10, 0, 0, time.UTC), r.PeakAt)
	require.Equal(t, 15*time.Minute, r.OverFor)
	require.Equal(t, 15*time.Minute, r.Longest)
	require.False(t, r.OverNow())
	require.True(t, r.WentOver())
	require.Equal(t, "DB CPU over 70% for 5 min", r.Problem())
}

func Test_AReadingWithoutSamplesIsNeverOver(t *testing.T) {
	r := entities.ReadingOf(entities.MetricResponse, 300, nil, 5*time.Minute)

	require.False(t, r.Known)
	require.False(t, entities.Health{Readings: []entities.Reading{r}}.Known())
	require.False(t, r.OverNow())
	require.False(t, r.WentOver())
}

func Test_TheLimitItselfCountsAsOver(t *testing.T) {
	r := entities.Reading{Metric: entities.MetricCPU, Limit: 70, Now: 70, Held: 70, Known: true}

	require.True(t, r.OverNow())
}

func Test_AMetricIsOverOnlyAfterStayingOverForFiveMinutes(t *testing.T) {
	brief := entities.ReadingOf(entities.MetricResponse, 300, samples(120, 120, 120, 450, 460, 470, 480), time.Minute)
	held := entities.ReadingOf(entities.MetricResponse, 300, samples(120, 120, 450, 460, 470, 480, 490), time.Minute)
	dipped := entities.ReadingOf(entities.MetricResponse, 300, samples(450, 460, 470, 120, 480, 490, 500, 510), time.Minute)

	require.False(t, brief.OverNow())
	require.False(t, brief.WentOver())
	require.True(t, held.OverNow())
	require.Equal(t, 450.0, held.Held)
	require.False(t, dipped.WentOver())
	require.Equal(t, 7*time.Minute, dipped.OverFor)
}

func Test_ASpikeNowWithoutFiveMinutesOverIsNotOver(t *testing.T) {
	r := entities.Reading{Metric: entities.MetricResponse, Limit: 300, Now: 900, Peak: 900, Held: 120, Known: true}

	require.False(t, r.OverNow())
	require.False(t, r.WentOver())
}

func Test_ProblemsListMetricsOverNowThenFailures(t *testing.T) {
	h := entities.Health{Readings: []entities.Reading{
		{Metric: entities.MetricResponse, Limit: 300, Now: 410, Held: 360, Known: true},
		{Metric: entities.MetricCPU, Limit: 70, Now: 12, Peak: 90, Longest: 10 * time.Minute, Known: true},
	}}

	problems := entities.Problems(h, entities.Delivery{FailedBuild: 9, FailedDeploy: 17})
	calm := entities.Problems(entities.Health{}, entities.Delivery{})

	require.Equal(t, []string{"Response p95 over 300 ms for 5 min", "Build #9 failed", "Deploy #17 to prod failed"}, problems)
	require.Empty(t, calm)
	require.Len(t, h.WentOver(), 2)
	require.Empty(t, h.Calm())
}
