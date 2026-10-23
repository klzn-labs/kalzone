package queuedfeed

import (
	"context"

	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/app/cron/usecases/executor"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
)

type Executor interface {
	Desired(ctx context.Context) (executor.Desired, error)
	Record(ctx context.Context, runs []entities.Run) error
	Done(ctx context.Context, commandID string) error
}

type Feed struct {
	executor Executor
}

func NewFeed(executor Executor) Feed {
	return Feed{executor: executor}
}

func (f Feed) Desired(ctx context.Context) ([]queuedapi.Cron, []queuedapi.Command, error) {
	desired, err := f.executor.Desired(ctx)
	if err != nil {
		return nil, nil, err
	}
	return mapAll(desired.Crons, toCronDTO), mapAll(desired.Commands, toCommandDTO), nil
}

func (f Feed) Record(ctx context.Context, runs []queuedapi.CronRun) error {
	return f.executor.Record(ctx, mapAll(runs, toRun))
}

func (f Feed) Done(ctx context.Context, commandID string) error {
	return f.executor.Done(ctx, commandID)
}

func toCronDTO(c executor.DesiredCron) queuedapi.Cron {
	return queuedapi.Cron{
		ID: c.ID, Name: c.Name, Environment: c.Environment, Service: c.Service, Schedule: c.Schedule, Target: c.Target,
		Payload: c.Payload, Timeout: c.Timeout.String(), Paused: c.Paused, SigningKey: c.SigningKey, PreviousKey: c.PreviousKey,
	}
}

func toCommandDTO(c entities.Command) queuedapi.Command {
	return queuedapi.Command{ID: c.ID, CronID: c.CronID, Kind: c.Kind}
}

func toRun(r queuedapi.CronRun) entities.Run {
	return entities.Run{
		ID: r.ID, CronID: r.CronID, Manual: r.Manual, Status: r.Status, ScheduledAt: r.ScheduledAt.UTC(),
		StartedAt: r.StartedAt.UTC(), FinishedAt: r.FinishedAt.UTC(), Attempts: r.Attempts, LastStatus: r.LastStatus, Error: r.Error,
	}
}

func mapAll[S any, T any](items []S, convert func(S) T) []T {
	result := make([]T, 0, len(items))
	for _, item := range items {
		result = append(result, convert(item))
	}
	return result
}
