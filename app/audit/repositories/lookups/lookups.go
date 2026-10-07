package lookups

import (
	"context"

	"github.com/adrianozp/platform/app/audit/entities"
	catalog "github.com/adrianozp/platform/app/catalog/entities"
	identity "github.com/adrianozp/platform/app/identity/entities"
	team "github.com/adrianozp/platform/app/team/entities"
)

type TeamFinder interface {
	Get(ctx context.Context, slug string) (team.Team, error)
}

type ServiceFinder interface {
	Get(ctx context.Context, slug string) (catalog.Service, error)
}

type UserFinder interface {
	FindByLogin(ctx context.Context, login string) (identity.User, error)
}

type Resolver struct {
	teams    TeamFinder
	services ServiceFinder
	users    UserFinder
}

func NewResolver(teams TeamFinder, services ServiceFinder, users UserFinder) Resolver {
	return Resolver{teams: teams, services: services, users: users}
}

func (r Resolver) Resolve(ctx context.Context, q entities.Query) (entities.Filter, error) {
	filter := entities.Filter{Environment: q.Environment, Before: q.Before, Limit: q.Limit}
	var err error
	if filter.TeamID, err = resolveIf(ctx, q.Team, r.teamID); err != nil {
		return entities.Filter{}, err
	}
	if filter.TargetID, err = resolveIf(ctx, q.Service, r.serviceID); err != nil {
		return entities.Filter{}, err
	}
	filter.ActorID, err = resolveIf(ctx, q.Actor, r.userID)
	return filter, err
}

func resolveIf(ctx context.Context, value string, resolve func(context.Context, string) (string, error)) (string, error) {
	if value == "" {
		return "", nil
	}
	return resolve(ctx, value)
}

func (r Resolver) teamID(ctx context.Context, slug string) (string, error) {
	t, err := r.teams.Get(ctx, slug)
	return t.ID, err
}

func (r Resolver) serviceID(ctx context.Context, slug string) (string, error) {
	s, err := r.services.Get(ctx, slug)
	return s.ID, err
}

func (r Resolver) userID(ctx context.Context, login string) (string, error) {
	u, err := r.users.FindByLogin(ctx, login)
	return u.ID, err
}
