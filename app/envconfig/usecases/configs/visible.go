package configs

import (
	"context"

	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
)

var errNotVisible = apperror.NotFound("service not found")

func (u ConfigUsecase) visible(ctx context.Context, a actor.Actor, serviceSlug string) error {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return err
	}
	if !a.Sees(service.OwnerTeamID) {
		return errNotVisible
	}
	return nil
}

func (u ConfigUsecase) CurrentFor(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.Revision, error) {
	if err := u.visible(ctx, a, serviceSlug); err != nil {
		return entities.Revision{}, err
	}
	return u.Current(ctx, serviceSlug, environment)
}

func (u ConfigUsecase) HistoryFor(ctx context.Context, a actor.Actor, serviceSlug, environment string, limit int) ([]entities.Revision, error) {
	if err := u.visible(ctx, a, serviceSlug); err != nil {
		return nil, err
	}
	return u.History(ctx, serviceSlug, environment, limit)
}

func (u ConfigUsecase) SecretsFor(ctx context.Context, a actor.Actor, serviceSlug, environment string) ([]entities.SecretInfo, error) {
	if err := u.visible(ctx, a, serviceSlug); err != nil {
		return nil, err
	}
	return u.Secrets(ctx, serviceSlug, environment)
}
