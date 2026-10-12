package releases

import (
	"context"

	"github.com/adrianozp/platform/app/release/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
)

var errNotVisible = apperror.NotFound("not found")

func (u ReleasesUsecase) ListFor(ctx context.Context, a actor.Actor, serviceSlug string, limit int) ([]entities.Release, error) {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return nil, err
	}
	if !a.Sees(service.OwnerTeamID) {
		return nil, errNotVisible
	}
	return u.List(ctx, serviceSlug, limit)
}

func (u ReleasesUsecase) GetFor(ctx context.Context, a actor.Actor, id string) (entities.Release, error) {
	rel, err := u.Get(ctx, id)
	if err != nil {
		return entities.Release{}, err
	}
	service, err := u.services.FindServiceByID(ctx, rel.ServiceID)
	if err != nil {
		return entities.Release{}, err
	}
	if !a.Sees(service.OwnerTeamID) {
		return entities.Release{}, errNotVisible
	}
	return rel, nil
}
