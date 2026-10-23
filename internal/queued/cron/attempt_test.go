package cron_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/cron"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
	"github.com/stretchr/testify/require"
)

func (w *world) runNow(t *testing.T, c queuedapi.Cron) queuedapi.CronRun {
	t.Helper()
	w.start(t, c)
	_, errs := w.scheduler.Apply([]queuedapi.Cron{c}, []queuedapi.Command{{ID: "k1", CronID: c.ID, Kind: "run"}})
	require.Empty(t, errs)
	return w.reported(t, 1)[0]
}

func Test_Attempt_RetriesA503UntilItSucceeds(t *testing.T) {
	w := newWorld(t, http.StatusServiceUnavailable, http.StatusOK)

	run := w.runNow(t, w.desired("@daily", false))

	require.Equal(t, cron.RunSucceeded, run.Status)
	require.Equal(t, 2, run.Attempts)
	require.Empty(t, run.Error)
	calls := w.target.received()
	require.Equal(t, []string{"1", "2"}, []string{calls[0].header.Get("Platform-Attempt"), calls[1].header.Get("Platform-Attempt")})
}

func Test_Attempt_A422FailsAtOnce(t *testing.T) {
	w := newWorld(t, http.StatusUnprocessableEntity)

	run := w.runNow(t, w.desired("@daily", false))

	require.Equal(t, cron.RunFailed, run.Status)
	require.Equal(t, 1, run.Attempts)
	require.Equal(t, http.StatusUnprocessableEntity, run.LastStatus)
	require.Contains(t, run.Error, "HTTP 422")
}

func Test_Attempt_TimesOutWhenTheBudgetRunsOut(t *testing.T) {
	w := newWorld(t)
	w.target.blocking = true
	c := w.desired("@daily", false)
	c.Timeout = "100ms"
	t.Cleanup(func() { close(w.target.release) })

	started := time.Now()
	run := w.runNow(t, c)

	require.Equal(t, cron.RunTimedOut, run.Status)
	require.Equal(t, 1, run.Attempts)
	require.Less(t, time.Since(started), time.Second)
}

func Test_Attempt_KeepsRetryingFailuresUntilTheTimeout(t *testing.T) {
	answers := []int{}
	for range 100 {
		answers = append(answers, http.StatusBadGateway)
	}
	w := newWorld(t, answers...)
	c := w.desired("@daily", false)
	c.Timeout = "100ms"

	run := w.runNow(t, c)

	require.Equal(t, cron.RunTimedOut, run.Status)
	require.Greater(t, run.Attempts, 1)
}
