package search

import (
	"cmp"
	"context"
	"slices"

	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
)

var errKey = apperror.Invalid("key must be a variable name in UPPER_SNAKE_CASE").WithField("key").
	WithHint("send ?key=LOG_LEVEL; the match is exact")

type Store interface {
	Search(ctx context.Context, key, environment string) ([]entities.Match, error)
}

type Directory interface {
	Services(ctx context.Context) ([]entities.ServiceRef, error)
	TeamID(ctx context.Context, teamSlug string) (string, error)
}

type EnvironmentLookup interface {
	EnvironmentExists(ctx context.Context, slug string) error
}

type Query struct {
	Key         string
	Environment string
	Team        string
}

type SearchUsecase struct {
	store        Store
	directory    Directory
	environments EnvironmentLookup
}

func NewSearchUsecase(store Store, directory Directory, environments EnvironmentLookup) SearchUsecase {
	return SearchUsecase{store: store, directory: directory, environments: environments}
}

func (u SearchUsecase) Find(ctx context.Context, a actor.Actor, q Query) ([]entities.Match, error) {
	if err := u.check(ctx, q); err != nil {
		return nil, err
	}
	visible, err := u.visibleServices(ctx, a, q.Team)
	if err != nil {
		return nil, err
	}
	matches, err := u.store.Search(ctx, q.Key, q.Environment)
	if err != nil {
		return nil, err
	}
	return within(matches, visible), nil
}

func (u SearchUsecase) check(ctx context.Context, q Query) error {
	if !entities.IsName(q.Key) {
		return errKey
	}
	if q.Environment == "" {
		return nil
	}
	return u.environments.EnvironmentExists(ctx, q.Environment)
}

func (u SearchUsecase) visibleServices(ctx context.Context, a actor.Actor, teamSlug string) (map[string]entities.ServiceRef, error) {
	teamID, err := u.grantedTeam(ctx, a, teamSlug)
	if err != nil {
		return nil, err
	}
	services, err := u.directory.Services(ctx)
	if err != nil {
		return nil, err
	}
	visible := map[string]entities.ServiceRef{}
	for _, s := range services {
		if a.Sees(s.OwnerTeamID) && (teamID == "" || s.OwnerTeamID == teamID) {
			visible[s.ID] = s
		}
	}
	return visible, nil
}

func (u SearchUsecase) grantedTeam(ctx context.Context, a actor.Actor, teamSlug string) (string, error) {
	if teamSlug == "" {
		return "", nil
	}
	teamID, err := u.directory.TeamID(ctx, teamSlug)
	if err != nil {
		return "", err
	}
	if !a.Sees(teamID) {
		return "", apperror.TeamNotGranted(teamSlug)
	}
	return teamID, nil
}

func within(matches []entities.Match, visible map[string]entities.ServiceRef) []entities.Match {
	result := []entities.Match{}
	for _, m := range matches {
		if service, seen := visible[m.ServiceID]; seen {
			result = append(result, shown(m, service))
		}
	}
	slices.SortFunc(result, func(a, b entities.Match) int {
		return cmp.Or(cmp.Compare(a.ServiceSlug, b.ServiceSlug), cmp.Compare(a.Environment, b.Environment))
	})
	return result
}

func shown(m entities.Match, service entities.ServiceRef) entities.Match {
	m.ServiceSlug = service.Slug
	if m.Kind == entities.MatchSecret {
		m.Value = ""
	}
	return m
}
