package teams

import (
	"context"
	"slices"

	"github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var errNotVisible = apperror.NotFound("team not found").WithHint("list teams with GET /api/v1/teams")

func (u TeamsUsecase) ListFor(ctx context.Context, a actor.Actor) ([]entities.Team, error) {
	list, err := u.store.List(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list, func(t entities.Team) bool { return !a.Sees(t.ID) }), nil
}

func (u TeamsUsecase) GetFor(ctx context.Context, a actor.Actor, teamSlug string) (entities.Team, error) {
	team, err := u.store.FindBySlug(ctx, teamSlug)
	if err != nil {
		return entities.Team{}, err
	}
	if !a.Sees(team.ID) {
		return entities.Team{}, errNotVisible
	}
	return team, nil
}
