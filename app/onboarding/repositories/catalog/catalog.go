package catalog

import (
	"context"

	catalog "github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/app/onboarding/entities"
)

type ServiceFinder interface {
	Get(ctx context.Context, slug string) (catalog.Service, error)
}

type Lookup struct {
	services ServiceFinder
}

func NewLookup(services ServiceFinder) Lookup {
	return Lookup{services: services}
}

func (l Lookup) FindService(ctx context.Context, slug string) (entities.Service, error) {
	s, err := l.services.Get(ctx, slug)
	return entities.Service{
		ID: s.ID, Slug: s.Slug, OwnerTeamID: s.OwnerTeamID, Repository: s.GitHubRepo,
		ImageRepository: s.ImageRepository, BuildWorkflow: s.BuildWorkflow,
	}, err
}
