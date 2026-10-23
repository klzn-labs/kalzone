package cron_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/cron"
	"github.com/klzn-labs/kalzone/internal/queued/signing"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
	"github.com/stretchr/testify/require"
)

const settle = 2 * time.Second

func (w *world) start(t *testing.T, c queuedapi.Cron) {
	t.Helper()
	_, errs := w.scheduler.Apply([]queuedapi.Cron{c}, nil)
	require.Empty(t, errs)
	w.armed(t)
}

func (w *world) armed(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool { return w.clock.waiting() == 1 }, settle, time.Millisecond)
}

func (w *world) reported(t *testing.T, count int) []queuedapi.CronRun {
	t.Helper()
	runs := []queuedapi.CronRun{}
	require.Eventually(t, func() bool {
		runs = append(runs, w.outbox.Drain()...)
		return len(runs) >= count
	}, settle, 5*time.Millisecond)
	return runs
}

func Test_Scheduler_FiresOncePerTickWithSignedHeaders(t *testing.T) {
	w := newWorld(t)
	w.start(t, w.desired("* * * * *", false))

	w.clock.Advance(30 * time.Second)
	runs := w.reported(t, 1)

	scheduled := time.Date(2026, 10, 23, 12, 1, 0, 0, time.UTC)
	require.Equal(t, queuedapi.CronRun{
		ID: cron.ScheduledRunID("c1", scheduled), CronID: "c1", Status: cron.RunSucceeded, ScheduledAt: scheduled,
		StartedAt: scheduled, FinishedAt: scheduled, Attempts: 1, LastStatus: http.StatusOK,
	}, runs[0])
	calls := w.target.received()
	require.Len(t, calls, 1)
	h := calls[0].header
	require.Equal(t, `{"full":true}`, calls[0].body)
	require.Equal(t, []string{"nightly", "2026-10-23T12:01:00Z", runs[0].ID, "1"},
		[]string{h.Get("Platform-Cron"), h.Get("Platform-Scheduled-At"), h.Get("Platform-Run-Id"), h.Get("Platform-Attempt")})
	require.NoError(t, signing.Verify(h.Get("Platform-Signature"), []string{"signing-key"}, []byte(calls[0].body), time.Now(), time.Minute))
}

func Test_Scheduler_SkipsATickWhileARunIsGoing(t *testing.T) {
	w := newWorld(t)
	w.target.blocking = true
	w.start(t, w.desired("* * * * *", false))

	w.clock.Advance(30 * time.Second)
	require.Eventually(t, func() bool { return len(w.target.received()) == 1 }, settle, time.Millisecond)
	w.armed(t)
	w.clock.Advance(time.Minute)
	skipped := w.reported(t, 1)
	close(w.target.release)
	finished := w.reported(t, 1)

	require.Equal(t, cron.RunSkipped, skipped[0].Status)
	require.Equal(t, time.Date(2026, 10, 23, 12, 2, 0, 0, time.UTC), skipped[0].ScheduledAt)
	require.Equal(t, cron.RunSucceeded, finished[0].Status)
	require.Len(t, w.target.received(), 1)
}

func Test_Scheduler_DoesNotCatchUpMissedTicks(t *testing.T) {
	w := newWorld(t)
	w.start(t, w.desired("* * * * *", false))

	w.clock.Advance(5 * time.Minute)
	runs := w.reported(t, 1)
	w.armed(t)
	w.clock.Advance(30 * time.Second)
	next := w.reported(t, 1)

	require.Len(t, runs, 1)
	require.Equal(t, time.Date(2026, 10, 23, 12, 1, 0, 0, time.UTC), runs[0].ScheduledAt)
	require.Equal(t, time.Date(2026, 10, 23, 12, 6, 0, 0, time.UTC), next[0].ScheduledAt)
	require.Len(t, w.target.received(), 2)
}

func Test_Scheduler_PausedCronOnlyRunsOnCommand(t *testing.T) {
	w := newWorld(t)
	w.start(t, w.desired("* * * * *", true))

	w.clock.Advance(30 * time.Second)
	w.armed(t)
	done, errs := w.scheduler.Apply([]queuedapi.Cron{w.desired("* * * * *", true)}, []queuedapi.Command{{ID: "k1", CronID: "c1", Kind: "run"}})
	runs := w.reported(t, 1)

	require.Empty(t, errs)
	require.Equal(t, []string{"k1"}, done)
	require.Equal(t, "k1", runs[0].ID)
	require.True(t, runs[0].Manual)
	require.Len(t, w.target.received(), 1)
}

func Test_Scheduler_FinishesCommandsOfUnknownCrons(t *testing.T) {
	w := newWorld(t)

	done, errs := w.scheduler.Apply(nil, []queuedapi.Command{{ID: "k1", CronID: "gone", Kind: "run"}})

	require.Empty(t, errs)
	require.Equal(t, []string{"k1"}, done)
	require.Empty(t, w.target.received())
}

func Test_Scheduler_ReportsAnInvalidCron(t *testing.T) {
	w := newWorld(t)
	bad := w.desired("@every 1m", false)

	_, errs := w.scheduler.Apply([]queuedapi.Cron{bad}, nil)

	require.Len(t, errs, 1)
	require.Zero(t, w.clock.waiting())
}
