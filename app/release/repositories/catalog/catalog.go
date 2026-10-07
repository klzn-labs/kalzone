package catalog

import (
	"context"

	catalog "github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/app/release/entities"
)

type ServiceFinder interface {
	Get(ctx context.Context, slug string) (catalog.Service, error)
	GetByID(ctx context.Context, id string) (catalog.Service, error)
	ForRepository(ctx context.Context, githubRepo string) ([]catalog.Service, error)
}

type Lookup struct {
	services ServiceFinder
}

func NewLookup(services ServiceFinder) Lookup {
	return Lookup{services: services}
}

func (l Lookup) FindService(ctx context.Context, slug string) (entities.ServiceRef, error) {
	s, err := l.services.Get(ctx, slug)
	return toRef(s), err
}

func (l Lookup) FindServiceByID(ctx context.Context, id string) (entities.ServiceRef, error) {
	s, err := l.services.GetByID(ctx, id)
	return toRef(s), err
}

func (l Lookup) ServicesForRepository(ctx context.Context, repository string) ([]entities.ServiceRef, error) {
	services, err := l.services.ForRepository(ctx, repository)
	if err != nil {
		return nil, err
	}
	refs := make([]entities.ServiceRef, 0, len(services))
	for _, s := range services {
		refs = append(refs, toRef(s))
	}
	return refs, nil
}

func toRef(s catalog.Service) entities.ServiceRef {
	return entities.ServiceRef{
		ID: s.ID, Slug: s.Slug, OwnerTeamID: s.OwnerTeamID, GitHubRepo: s.GitHubRepo,
		ImageRepository: s.ImageRepository, BuildWorkflow: s.BuildWorkflow, AutoBuild: s.AutoBuild,
	}
}
