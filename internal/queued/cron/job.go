package cron

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/klzn-labs/kalzone/pkg/queuedapi"
)

type job struct {
	mu       sync.Mutex
	cron     Cron
	runs     context.Context
	executor executor
	outbox   *Outbox
	running  atomic.Bool
	wake     chan struct{}
	cancel   context.CancelFunc
	loop     sync.WaitGroup
}

func newJob(c Cron, runs context.Context, e executor, outbox *Outbox) *job {
	return &job{cron: c, runs: runs, executor: e, outbox: outbox, wake: make(chan struct{}, 1)}
}

func (j *job) start() {
	ctx, cancel := context.WithCancel(j.runs)
	j.cancel = cancel
	j.loop.Add(1)
	go j.run(ctx)
}

func (j *job) stop() {
	j.cancel()
	j.loop.Wait()
}

func (j *job) update(c Cron) {
	j.mu.Lock()
	changed := !j.cron.same(c)
	j.cron = c
	j.mu.Unlock()
	if changed {
		j.nudge()
	}
}

func (j *job) nudge() {
	select {
	case j.wake <- struct{}{}:
	default:
	}
}

func (j *job) snapshot() Cron {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.cron
}

func (j *job) run(ctx context.Context) {
	defer j.loop.Done()
	clock := j.executor.clock
	for ctx.Err() == nil {
		now := clock.Now()
		next := j.snapshot().Schedule.Next(now)
		select {
		case <-ctx.Done():
		case <-j.wake:
		case <-clock.After(next.Sub(now)):
			j.tick(next)
		}
	}
}

func (j *job) tick(scheduled time.Time) {
	c := j.snapshot()
	if c.Paused {
		return
	}
	j.launch(c, scheduledRun(c, scheduled))
}

func (j *job) runNow(commandID string) {
	c := j.snapshot()
	j.launch(c, manualRun(c, commandID, j.executor.clock.Now()))
}

func (j *job) launch(c Cron, run queuedapi.CronRun) {
	if !j.running.CompareAndSwap(false, true) {
		j.outbox.Add(j.executor.skipped(run))
		return
	}
	go func() {
		defer j.running.Store(false)
		j.outbox.Add(j.executor.execute(j.runs, c, run))
	}()
}
