package cron

import (
	"sync"

	"github.com/klzn-labs/kalzone/pkg/queuedapi"
)

type Outbox struct {
	mu    sync.Mutex
	limit int
	runs  []queuedapi.CronRun
}

func NewOutbox(limit int) *Outbox {
	return &Outbox{limit: limit}
}

func (o *Outbox) Add(run queuedapi.CronRun) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.runs = o.newest(append(o.runs, run))
}

func (o *Outbox) Drain() []queuedapi.CronRun {
	o.mu.Lock()
	defer o.mu.Unlock()
	runs := o.runs
	o.runs = nil
	return runs
}

func (o *Outbox) Restore(runs []queuedapi.CronRun) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.runs = o.newest(append(append([]queuedapi.CronRun{}, runs...), o.runs...))
}

func (o *Outbox) newest(runs []queuedapi.CronRun) []queuedapi.CronRun {
	if len(runs) <= o.limit {
		return runs
	}
	return runs[len(runs)-o.limit:]
}
