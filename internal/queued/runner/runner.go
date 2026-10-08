package runner

import (
	"context"
	"log"
	"time"

	"github.com/adrianozp/platform/pkg/queuedapi"
)

type ControlPlane interface {
	Desired(ctx context.Context) (queuedapi.Desired, error)
	Report(ctx context.Context, stats []queuedapi.Stats) error
	Done(ctx context.Context, commandID string) error
}

type Applier interface {
	Apply(ctx context.Context, desired queuedapi.Desired) ([]string, []error)
}

type StatsSource interface {
	Collect(ctx context.Context) []queuedapi.Stats
}

type Loop struct {
	controlPlane ControlPlane
	applier      Applier
	stats        StatsSource
	poll         time.Duration
	report       time.Duration
}

func New(cp ControlPlane, applier Applier, stats StatsSource, poll, report time.Duration) Loop {
	return Loop{controlPlane: cp, applier: applier, stats: stats, poll: poll, report: report}
}

func (l Loop) Run(ctx context.Context) {
	poll, report := time.NewTicker(l.poll), time.NewTicker(l.report)
	defer poll.Stop()
	defer report.Stop()
	l.PollOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
			l.PollOnce(ctx)
		case <-report.C:
			l.ReportOnce(ctx)
		}
	}
}

func (l Loop) PollOnce(ctx context.Context) {
	desired, err := l.controlPlane.Desired(ctx)
	if err != nil {
		log.Printf("desired state unavailable, keeping the current queues: %v", err)
		return
	}
	done, errs := l.applier.Apply(ctx, desired)
	for _, err := range errs {
		log.Printf("applying desired state: %v", err)
	}
	for _, id := range done {
		if err := l.controlPlane.Done(ctx, id); err != nil {
			log.Printf("confirming command %s: %v", id, err)
		}
	}
}

func (l Loop) ReportOnce(ctx context.Context) {
	if err := l.controlPlane.Report(ctx, l.stats.Collect(ctx)); err != nil {
		log.Printf("reporting stats: %v", err)
	}
}
