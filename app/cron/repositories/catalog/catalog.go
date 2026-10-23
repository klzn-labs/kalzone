package catalog

import (
	"context"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/cron/usecases/crons"
	team "github.com/klzn-labs/kalzone/app/team/entities"
)

type ServiceFinder interface {
	Get(ctx context.Context, slug string) (catalog.Service, error)
	GetByID(ctx context.Context, id string) (catalog.Service, error)
}

type EnvironmentFinder interface {
	Get(ctx context.Context, slug string) (catalog.Environment, error)
}

type TeamFinder interface {
	Get(ctx context.Context, slug string) (team.Team, error)
}

type Lookup struct {
	services     ServiceFinder
	environments EnvironmentFinder
	teams        TeamFinder
}

func NewLookup(services ServiceFinder, environments EnvironmentFinder, teams TeamFinder) Lookup {
	return Lookup{services: services, environments: environments, teams: teams}
}

func (l Lookup) FindService(ctx context.Context, slug string) (crons.ServiceRef, error) {
	s, err := l.services.Get(ctx, slug)
	return refOf(s), err
}

func (l Lookup) FindServiceByID(ctx context.Context, id string) (crons.ServiceRef, error) {
	s, err := l.services.GetByID(ctx, id)
	return refOf(s), err
}

func refOf(s catalog.Service) crons.ServiceRef {
	return crons.ServiceRef{ID: s.ID, Slug: s.Slug, OwnerTeamID: s.OwnerTeamID, Decommissioning: s.Decommissioning()}
}

func (l Lookup) EnvironmentExists(ctx context.Context, slug string) error {
	_, err := l.environments.Get(ctx, slug)
	return err
}

func (l Lookup) FindTeam(ctx context.Context, slug string) (string, error) {
	t, err := l.teams.Get(ctx, slug)
	return t.ID, err
}

func (l Lookup) SlugOf(ctx context.Context, id string) (string, error) {
	s, err := l.services.GetByID(ctx, id)
	return s.Slug, err
}
