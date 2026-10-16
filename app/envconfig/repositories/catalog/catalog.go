package catalog

import (
	"context"
	"errors"

	catalog "github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/pkg/apperror"
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
	return entities.ServiceRef{ID: s.ID, Slug: s.Slug, OwnerTeamID: s.OwnerTeamID}, err
}

func (l Lookup) ServiceExists(ctx context.Context, id string) (bool, error) {
	_, err := l.services.GetByID(ctx, id)
	if errors.Is(err, apperror.NotFound("")) {
		return false, nil
	}
	return err == nil, err
}

func (l Lookup) EnvironmentExists(ctx context.Context, slug string) error {
	_, err := l.environments.Get(ctx, slug)
	return err
}
