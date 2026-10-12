package query

import (
	"context"

	"github.com/adrianozp/platform/app/audit/entities"
	"github.com/adrianozp/platform/pkg/actor"
)

func (u QueryUsecase) ListFor(ctx context.Context, a actor.Actor, filter entities.Filter) ([]entities.Event, error) {
	if !a.ViaToken() {
		return u.List(ctx, filter)
	}
	if len(a.Token.TeamIDs) == 0 {
		return []entities.Event{}, nil
	}
	filter.TeamIDs = a.Token.TeamIDs
	return u.List(ctx, filter)
}
