package users

import (
	"context"
	"slices"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var errNotVisible = apperror.NotFound("user not found").WithHint("you see people who share a team with you")

func (u UsersUsecase) ListFor(ctx context.Context, a actor.Actor) ([]entities.User, error) {
	ids, all, err := u.visibleIDs(ctx, a)
	switch {
	case err != nil:
		return nil, err
	case all:
		return u.users.List(ctx)
	}
	return u.users.FindByIDs(ctx, ids)
}

func (u UsersUsecase) GetFor(ctx context.Context, a actor.Actor, login string) (entities.User, error) {
	user, err := u.users.FindByLogin(ctx, login)
	if err != nil {
		return entities.User{}, err
	}
	ids, all, err := u.visibleIDs(ctx, a)
	if err != nil {
		return entities.User{}, err
	}
	if !all && !slices.Contains(ids, user.ID) {
		return entities.User{}, errNotVisible
	}
	return user, nil
}

func (u UsersUsecase) visibleIDs(ctx context.Context, a actor.Actor) ([]string, bool, error) {
	teams, all := a.TeamScope()
	if all {
		return nil, true, nil
	}
	ids, err := u.members.UserIDsIn(ctx, teams)
	return append(ids, a.UserID), false, err
}
