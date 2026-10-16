package releases

import (
	"context"
	"log"

	"github.com/adrianozp/platform/app/release/entities"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/uuid"
)

func (u ReleasesUsecase) share(ctx context.Context, built entities.ServiceRef, rel entities.Release) {
	siblings, err := u.siblings(ctx, built)
	if err != nil {
		log.Printf("sharing release %s of %s: %v", rel.ID, built.Slug, err)
		return
	}
	for _, sibling := range siblings {
		shared, err := u.store.Create(ctx, rel.SharedWith(sibling, uuid.NewUUID(), clock.Now().UTC()))
		if err != nil {
			log.Printf("sharing release %s of %s with %s: %v", rel.ID, built.Slug, sibling.Slug, err)
			continue
		}
		u.publish(ctx, GitHubActor, "release.shared", sibling, shared)
	}
}

func (u ReleasesUsecase) siblings(ctx context.Context, service entities.ServiceRef) ([]entities.ServiceRef, error) {
	candidates, err := u.services.ServicesForRepository(ctx, service.GitHubRepo)
	if err != nil {
		return nil, err
	}
	result := []entities.ServiceRef{}
	for _, candidate := range candidates {
		if candidate.SharesImageWith(service) && !candidate.Decommissioning {
			result = append(result, candidate)
		}
	}
	return result, nil
}
