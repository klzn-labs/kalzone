package deployments

import (
	"context"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
)

var errNotVisible = apperror.NotFound("not found")

func (u DeploymentsUsecase) HistoryFor(ctx context.Context, a actor.Actor, serviceSlug, environment string, limit int) ([]entities.Deployment, error) {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return nil, err
	}
	if !a.Sees(service.OwnerTeamID) {
		return nil, errNotVisible
	}
	return u.History(ctx, serviceSlug, environment, limit)
}

func (u DeploymentsUsecase) GetFor(ctx context.Context, a actor.Actor, id string) (entities.Deployment, []entities.Event, error) {
	d, logged, err := u.Get(ctx, id)
	if err != nil {
		return entities.Deployment{}, nil, err
	}
	service, err := u.services.FindServiceByID(ctx, d.ServiceID)
	if err != nil {
		return entities.Deployment{}, nil, err
	}
	if !a.Sees(service.OwnerTeamID) {
		return entities.Deployment{}, nil, errNotVisible
	}
	return d, logged, nil
}
