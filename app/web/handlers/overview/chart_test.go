package overview

import (
	"testing"
	"time"

	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/stretchr/testify/require"
)

func finished(status deploy.Status, at time.Time, took time.Duration) deploy.Deployment {
	started := at.Add(-took)
	return deploy.Deployment{Status: status, StartedAt: &started, FinishedAt: &at}
}

func Test_ChartCountsFourteenDaysEndingToday(t *testing.T) {
	today := time.Date(2026, 10, 21, 9, 41, 0, 0, time.UTC)
	ds := []deploy.Deployment{
		finished(deploy.StatusSucceeded, time.Date(2026, 10, 8, 5, 0, 0, 0, time.UTC), 2*time.Minute),
		finished(deploy.StatusSucceeded, time.Date(2026, 10, 16, 4, 0, 0, 0, time.UTC), 3*time.Minute),
		finished(deploy.StatusFailed, time.Date(2026, 10, 16, 5, 0, 0, 0, time.UTC), time.Minute),
		finished(deploy.StatusSucceeded, time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC), 4*time.Minute),
		finished(deploy.StatusSucceeded, time.Date(2026, 10, 7, 23, 0, 0, 0, time.UTC), 10*time.Second),
		finished(deploy.StatusSucceeded, time.Date(2026, 10, 7, 22, 0, 0, 0, time.UTC), 20*time.Second),
	}

	c := chartOf(ds, today)

	require.Len(t, c.Days, 14)
	require.Equal(t, "8 Oct", c.Days[0].Label)
	require.Equal(t, "21 Oct", c.Days[13].Label)
	require.Equal(t, []int{1, 2, 1}, []int{c.Days[0].Count, c.Days[8].Count, c.Days[13].Count})
	require.Equal(t, 4, c.Total)
	require.Equal(t, 1, c.Failed)
	require.Equal(t, 3*time.Minute, c.Median)
	require.True(t, c.Days[13].ShowLabel)
}

func Test_AxisTopsOutAboveTheHighestDayInRoundSteps(t *testing.T) {
	for _, tc := range []struct{ highest, step, top int }{{0, 1, 1}, {3, 1, 4}, {4, 2, 6}, {10, 5, 15}, {23, 10, 30}} {
		step, top := axis([]int{0, tc.highest})
		require.Equal(t, []int{tc.step, tc.top}, []int{step, top}, "highest %d", tc.highest)
	}
}

func Test_BarHeightIsTheShareOfTheTopTick(t *testing.T) {
	ticks, days := scale([]int{0, 5, 10}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))

	require.Equal(t, []int{0, 33, 66}, []int{days[0].Height, days[1].Height, days[2].Height})
	require.Equal(t, Tick{Value: 15, Bottom: 100}, ticks[len(ticks)-1])
}
