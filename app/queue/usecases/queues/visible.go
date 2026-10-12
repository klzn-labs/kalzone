package queues

import (
	"context"
	"slices"

	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
)

var errNotVisible = apperror.NotFound("queue not found")

func (u QueuesUsecase) GetFor(ctx context.Context, a actor.Actor, environment, name string) (entities.Queue, error) {
	q, err := u.store.Get(ctx, environment, name)
	if err != nil {
		return entities.Queue{}, err
	}
	if !a.Sees(q.TeamID) {
		return entities.Queue{}, errNotVisible
	}
	return q, nil
}

func (u QueuesUsecase) ListFor(ctx context.Context, a actor.Actor, environment, teamSlug string) ([]entities.Queue, error) {
	list, err := u.List(ctx, environment, teamSlug)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list, func(q entities.Queue) bool { return !a.Sees(q.TeamID) }), nil
}

func (u QueuesUsecase) VersionsFor(ctx context.Context, a actor.Actor, environment, name string) ([]entities.Version, error) {
	if _, err := u.GetFor(ctx, a, environment, name); err != nil {
		return nil, err
	}
	return u.Versions(ctx, environment, name)
}

func (u QueuesUsecase) StatsFor(ctx context.Context, a actor.Actor, environment, name string) (entities.Stats, bool, error) {
	if _, err := u.GetFor(ctx, a, environment, name); err != nil {
		return entities.Stats{}, false, err
	}
	return u.Stats(ctx, environment, name)
}
