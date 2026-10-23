package queued

import (
	"context"

	"github.com/klzn-labs/kalzone/pkg/queuedapi"
)

type queueApplier interface {
	Apply(ctx context.Context, desired queuedapi.Desired) ([]string, []error)
}

type cronApplier interface {
	Apply(crons []queuedapi.Cron, commands []queuedapi.Command) ([]string, []error)
}

type applier struct {
	queues queueApplier
	crons  cronApplier
}

func (a applier) Apply(ctx context.Context, desired queuedapi.Desired) ([]string, []error) {
	cronCommands, queueCommands := splitByCron(desired.Commands)
	cronDone, cronErrs := a.crons.Apply(desired.Crons, cronCommands)
	desired.Commands = queueCommands
	done, errs := a.queues.Apply(ctx, desired)
	return append(done, cronDone...), append(errs, cronErrs...)
}

func splitByCron(commands []queuedapi.Command) ([]queuedapi.Command, []queuedapi.Command) {
	crons, queues := []queuedapi.Command{}, []queuedapi.Command{}
	for _, c := range commands {
		if c.CronID != "" {
			crons = append(crons, c)
			continue
		}
		queues = append(queues, c)
	}
	return crons, queues
}
