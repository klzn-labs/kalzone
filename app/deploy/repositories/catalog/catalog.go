package catalog

import (
	"context"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/deploy/entities"
)

type ServiceFinder interface {
	Get(ctx context.Context, slug string) (catalog.Service, error)
	GetByID(ctx context.Context, id string) (catalog.Service, error)
}

type EnvironmentFinder interface {
	Get(ctx context.Context, slug string) (catalog.Environment, error)
}

type Lookup struct {
	services     ServiceFinder
	environments EnvironmentFinder
}

func NewLookup(services ServiceFinder, environments EnvironmentFinder) Lookup {
	return Lookup{services: services, environments: environments}
}

func (l Lookup) FindService(ctx context.Context, slug string) (entities.ServiceRef, error) {
	s, err := l.services.Get(ctx, slug)
	return serviceRef(s), err
}

func (l Lookup) FindServiceByID(ctx context.Context, id string) (entities.ServiceRef, error) {
	s, err := l.services.GetByID(ctx, id)
	return serviceRef(s), err
}

func serviceRef(s catalog.Service) entities.ServiceRef {
	return entities.ServiceRef{ID: s.ID, Slug: s.Slug, OwnerTeamID: s.OwnerTeamID, AutoDeployDev: s.AutoDeployDev, Decommissioning: s.Decommissioning()}
}

func (l Lookup) EnvironmentExists(ctx context.Context, slug string) error {
	_, err := l.environments.Get(ctx, slug)
	return err
}
