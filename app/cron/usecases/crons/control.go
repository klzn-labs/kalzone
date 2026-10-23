package crons

import (
	"context"

	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

func (u CronsUsecase) SetPaused(ctx context.Context, a actor.Actor, environment, name string, paused bool) (entities.Cron, error) {
	c, gate, err := u.operable(ctx, a, environment, name)
	if err != nil {
		return entities.Cron{}, err
	}
	if err := u.store.SetPaused(ctx, c.ID, paused); err != nil {
		return entities.Cron{}, err
	}
	c.Paused = paused
	u.publish(ctx, a, gate, map[bool]string{true: "cron.paused", false: "cron.resumed"}[paused], c, nil)
	return c, nil
}

func (u CronsUsecase) Run(ctx context.Context, a actor.Actor, environment, name string) (entities.Command, error) {
	c, gate, err := u.operable(ctx, a, environment, name)
	if err != nil {
		return entities.Command{}, err
	}
	command := entities.Command{ID: uuid.NewUUID(), CronID: c.ID, Kind: entities.CommandRun, CreatedAt: clock.Now().UTC()}
	if err := u.store.AddCommand(ctx, command); err != nil {
		return entities.Command{}, err
	}
	u.publish(ctx, a, gate, "cron.run_requested", c, map[string]any{"run": command.ID})
	return command, nil
}
