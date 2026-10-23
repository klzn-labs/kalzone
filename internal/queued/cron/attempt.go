package cron

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/dispatch"
	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
)

var DefaultBackoff = model.Backoff{Kind: model.BackoffExponential, Initial: time.Second, Max: 30 * time.Second}

type executor struct {
	sender  sender
	backoff model.Backoff
	clock   Clock
}

func (e executor) execute(ctx context.Context, c Cron, run queuedapi.CronRun) queuedapi.CronRun {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	run.StartedAt = e.clock.Now().UTC()
	for attempt := 1; ; attempt++ {
		run.Attempts = attempt
		result := e.sender.send(ctx, c, run)
		run.LastStatus, run.Error = result.status, result.describe()
		verdict, after := dispatch.Outcome(result.status, result.err, result.header)
		if verdict != dispatch.Retry {
			return e.finish(run, verdict)
		}
		if !wait(ctx, dispatch.Delay(e.backoff, attempt, after, rand.Float64)) {
			return e.finished(run, RunTimedOut)
		}
	}
}

func (e executor) finish(run queuedapi.CronRun, verdict dispatch.Verdict) queuedapi.CronRun {
	if verdict == dispatch.Ack {
		run.Error = ""
		return e.finished(run, RunSucceeded)
	}
	return e.finished(run, RunFailed)
}

func (e executor) finished(run queuedapi.CronRun, status string) queuedapi.CronRun {
	run.Status, run.FinishedAt = status, e.clock.Now().UTC()
	return run
}

func (e executor) skipped(run queuedapi.CronRun) queuedapi.CronRun {
	run.StartedAt = e.clock.Now().UTC()
	run.Error = "the previous run was still going"
	return e.finished(run, RunSkipped)
}

func wait(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
