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
	require.False(t, r.OverNow())
	require.True(t, r.WentOver())
	require.Equal(t, "DB CPU over 70%", r.Problem())
}

func Test_AReadingWithoutSamplesIsNeverOver(t *testing.T) {
	r := entities.ReadingOf(entities.MetricResponse, 300, nil, 5*time.Minute)

	require.False(t, r.Known)
	require.False(t, entities.Health{Readings: []entities.Reading{r}}.Known())
	require.False(t, r.OverNow())
	require.False(t, r.WentOver())
}

func Test_TheLimitItselfCountsAsOver(t *testing.T) {
	r := entities.Reading{Metric: entities.MetricCPU, Limit: 70, Now: 70, Known: true}

	require.True(t, r.OverNow())
}

func Test_ProblemsListMetricsOverNowThenFailures(t *testing.T) {
	h := entities.Health{Readings: []entities.Reading{
		{Metric: entities.MetricResponse, Limit: 300, Now: 410, Known: true},
		{Metric: entities.MetricCPU, Limit: 70, Now: 12, Peak: 90, Known: true},
	}}

	problems := entities.Problems(h, entities.Delivery{FailedBuild: 9, FailedDeploy: 17})
	calm := entities.Problems(entities.Health{}, entities.Delivery{})

	require.Equal(t, []string{"Response p95 over 300 ms", "Build #9 failed", "Deploy #17 to prod failed"}, problems)
	require.Empty(t, calm)
	require.Len(t, h.WentOver(), 2)
	require.Empty(t, h.Calm())
}
