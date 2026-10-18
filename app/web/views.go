package web

import (
	"context"

	"github.com/klzn-labs/kalzone/app/authz"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	envconfig "github.com/klzn-labs/kalzone/app/envconfig/entities"
	identity "github.com/klzn-labs/kalzone/app/identity/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type TeamFinder interface {
	FindByIDs(ctx context.Context, ids []string) ([]team.Team, error)
}

type MembershipSource interface {
	MembershipsOf(ctx context.Context, userID string) ([]team.Membership, error)
}

type ConfigReader interface {
	Current(ctx context.Context, serviceSlug, environment string) (envconfig.Revision, error)
}

type LiveReader interface {
	Live(ctx context.Context, serviceSlug, environment string) (deploy.Deployment, bool, error)
}

type EnvironmentLister interface {
	List(ctx context.Context) ([]catalog.Environment, error)
}

type UserFinder interface {
	FindByIDs(ctx context.Context, ids []string) ([]identity.User, error)
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type Views struct {
	teams        TeamFinder
	memberships  MembershipSource
	configs      ConfigReader
	live         LiveReader
	environments EnvironmentLister
	users        UserFinder
	authz        Authorizer
}

func NewViews(teams TeamFinder, memberships MembershipSource, configs ConfigReader, live LiveReader, environments EnvironmentLister, users UserFinder, authz Authorizer) Views {
	return Views{teams: teams, memberships: memberships, configs: configs, live: live, environments: environments, users: users, authz: authz}
}

func (v Views) Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) bool {
	return v.authz.Can(ctx, a, action, r) == nil
}

func (v Views) Environments(ctx context.Context) ([]catalog.Environment, error) {
	return v.environments.List(ctx)
}

func (v Views) UsersByID(ctx context.Context, ids []string) (map[string]identity.User, error) {
	users, err := v.users.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]identity.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}
	return byID, nil
}

func (v Views) TeamsByID(ctx context.Context, ids []string) (map[string]team.Team, error) {
	teams, err := v.teams.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]team.Team, len(teams))
	for _, t := range teams {
		byID[t.ID] = t
	}
	return byID, nil
}
