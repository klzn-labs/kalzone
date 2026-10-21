package deployments

import (
	"context"
	"errors"
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

func (u DeploymentsUsecase) FinishedFor(ctx context.Context, a actor.Actor, environment string, since time.Time) ([]entities.Deployment, error) {
	finished, err := u.store.FinishedSince(ctx, environment, since)
	if err != nil {
		return nil, err
	}
	sees, result := map[string]bool{}, []entities.Deployment{}
	for _, d := range finished {
		visible, err := u.seesService(ctx, a, d.ServiceID, sees)
		if err != nil {
			return nil, err
		}
		if visible {
			result = append(result, d)
		}
	}
	return result, nil
}

func (u DeploymentsUsecase) seesService(ctx context.Context, a actor.Actor, serviceID string, known map[string]bool) (bool, error) {
	if visible, ok := known[serviceID]; ok {
		return visible, nil
	}
	service, err := u.services.FindServiceByID(ctx, serviceID)
	if errors.Is(err, apperror.NotFound("")) {
		known[serviceID] = false
		return false, nil
	}
	if err != nil {
		return false, err
	}
	known[serviceID] = a.Sees(service.OwnerTeamID)
	return known[serviceID], nil
}
