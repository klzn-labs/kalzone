package cron

import (
	"context"
	"log"
	"sync"

	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
)

type Options struct {
	Client  HTTPDoer
	Clock   Clock
	Backoff model.Backoff
	Outbox  *Outbox
}

type Scheduler struct {
	ctx      context.Context
	executor executor
	outbox   *Outbox
	mu       sync.Mutex
	jobs     map[string]*job
}

func New(ctx context.Context, o Options) *Scheduler {
	e := executor{sender: sender{client: o.Client}, backoff: o.Backoff, clock: o.Clock}
	return &Scheduler{ctx: ctx, executor: e, outbox: o.Outbox, jobs: map[string]*job{}}
}

func (s *Scheduler) Apply(crons []queuedapi.Cron, commands []queuedapi.Command) ([]string, []error) {
	errs := s.applyCrons(crons)
	done := make([]string, 0, len(commands))
	for _, c := range commands {
		s.runNow(c)
		done = append(done, c.ID)
	}
	return done, errs
}

func (s *Scheduler) applyCrons(crons []queuedapi.Cron) []error {
	errs := []error{}
	seen := map[string]bool{}
	for _, raw := range crons {
		seen[raw.ID] = true
		c, err := FromAPI(raw)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		s.upsert(c)
	}
	s.stopMissing(seen)
	return errs
}

func (s *Scheduler) upsert(c Cron) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, known := s.jobs[c.ID]; known {
		j.update(c)
		return
	}
	j := newJob(c, s.ctx, s.executor, s.outbox)
	j.start()
	s.jobs[c.ID] = j
}

func (s *Scheduler) stopMissing(seen map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, j := range s.jobs {
		if !seen[id] {
			j.stop()
			delete(s.jobs, id)
		}
	}
}

func (s *Scheduler) runNow(c queuedapi.Command) {
	s.mu.Lock()
	j, known := s.jobs[c.CronID]
	s.mu.Unlock()
	if !known || c.Kind != "run" {
		log.Printf("command %s skipped: cron %s is not running here or kind %q is unknown", c.ID, c.CronID, c.Kind)
		return
	}
	j.runNow(c.ID)
}

func (s *Scheduler) StopAll() {
	s.stopMissing(map[string]bool{})
}
