package catalog

import (
	"context"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var errTeamNotFound = apperror.NotFound("team not found")

type ServiceFinder interface {
	Get(ctx context.Context, slug string) (catalog.Service, error)
	GetByID(ctx context.Context, id string) (catalog.Service, error)
}

type TeamFinder interface {
	Get(ctx context.Context, slug string) (team.Team, error)
	FindByIDs(ctx context.Context, ids []string) ([]team.Team, error)
}

type ProtectionReader interface {
	IsProtected(ctx context.Context, environment string) (bool, error)
}

type Lookup struct {
	services     ServiceFinder
	teams        TeamFinder
	environments ProtectionReader
}

func NewLookup(services ServiceFinder, teams TeamFinder, environments ProtectionReader) Lookup {
	return Lookup{services: services, teams: teams, environments: environments}
}

func (l Lookup) FindService(ctx context.Context, slug string) (databases.ServiceRef, error) {
	s, err := l.services.Get(ctx, slug)
	return refOf(s), err
}

func (l Lookup) FindServiceByID(ctx context.Context, id string) (databases.ServiceRef, error) {
	s, err := l.services.GetByID(ctx, id)
	return refOf(s), err
}

func refOf(s catalog.Service) databases.ServiceRef {
	return databases.ServiceRef{ID: s.ID, Slug: s.Slug, OwnerTeamID: s.OwnerTeamID}
}

func (l Lookup) FindTeam(ctx context.Context, slug string) (string, error) {
	t, err := l.teams.Get(ctx, slug)
	return t.ID, err
}

func (l Lookup) TeamSlug(ctx context.Context, id string) (string, error) {
	found, err := l.teams.FindByIDs(ctx, []string{id})
	if err != nil {
		return "", err
	}
	if len(found) == 0 {
		return "", errTeamNotFound
	}
	return found[0].Slug, nil
}

func (l Lookup) IsProtected(ctx context.Context, environment string) (bool, error) {
	return l.environments.IsProtected(ctx, environment)
}
