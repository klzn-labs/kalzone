package crons

import (
	"context"
	"slices"

	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var errNotVisible = apperror.NotFound("cron not found").WithHint("list crons with GET /api/v1/crons")

func (u CronsUsecase) GetFor(ctx context.Context, a actor.Actor, environment, name string) (entities.Cron, error) {
	c, err := u.store.Get(ctx, environment, name)
	if err != nil {
		return entities.Cron{}, err
	}
	if !a.Sees(c.TeamID) {
		return entities.Cron{}, errNotVisible
	}
	return c, nil
}

func (u CronsUsecase) ListFor(ctx context.Context, a actor.Actor, environment, teamSlug string) ([]entities.Cron, error) {
	list, err := u.list(ctx, environment, teamSlug)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list, func(c entities.Cron) bool { return !a.Sees(c.TeamID) }), nil
}

func (u CronsUsecase) list(ctx context.Context, environment, teamSlug string) ([]entities.Cron, error) {
	if teamSlug == "" {
		return u.store.List(ctx, environment, "")
	}
	teamID, err := u.catalog.FindTeam(ctx, teamSlug)
	if err != nil {
		return nil, err
	}
	return u.store.List(ctx, environment, teamID)
}

func (u CronsUsecase) RunsFor(ctx context.Context, a actor.Actor, environment, name string) ([]entities.Run, error) {
	c, err := u.GetFor(ctx, a, environment, name)
	if err != nil {
		return nil, err
	}
	return u.store.Runs(ctx, c.ID, entities.KeptRuns)
}

func (u CronsUsecase) LatestRun(ctx context.Context, c entities.Cron) (entities.Run, bool, error) {
	runs, err := u.store.Runs(ctx, c.ID, 1)
	if err != nil || len(runs) == 0 {
		return entities.Run{}, false, err
	}
	return runs[0], true, nil
}
