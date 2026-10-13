package queues

import (
	"context"
	"slices"
	"strconv"

	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/page"
)

var errNotVisible = apperror.NotFound("queue not found").WithHint("list queues with GET /api/v1/queues")

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

func (u QueuesUsecase) VersionPageFor(ctx context.Context, a actor.Actor, environment, name string, p page.Request) (page.Page[entities.Version], error) {
	q, err := u.GetFor(ctx, a, environment, name)
	if err != nil {
		return page.Page[entities.Version]{}, err
	}
	before, err := p.Number()
	if err != nil {
		return page.Page[entities.Version]{}, err
	}
	list, err := u.store.Versions(ctx, q.ID, before, p.Fetch())
	return page.Cut(list, p, versionKey), err
}

func versionKey(v entities.Version) string {
	return strconv.Itoa(v.Number)
}

func (u QueuesUsecase) StatsFor(ctx context.Context, a actor.Actor, environment, name string) (entities.Stats, bool, error) {
	if _, err := u.GetFor(ctx, a, environment, name); err != nil {
		return entities.Stats{}, false, err
	}
	return u.Stats(ctx, environment, name)
}
