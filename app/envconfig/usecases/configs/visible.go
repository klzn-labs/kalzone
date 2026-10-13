package configs

import (
	"context"
	"strconv"

	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/page"
)

var errNotVisible = apperror.NotFound("service not found").WithHint("list services with GET /api/v1/services")

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

func (u ConfigUsecase) RevisionsFor(ctx context.Context, a actor.Actor, serviceSlug, environment string, p page.Request) (page.Page[entities.Revision], error) {
	if err := u.visible(ctx, a, serviceSlug); err != nil {
		return page.Page[entities.Revision]{}, err
	}
	t, err := u.resolve(ctx, serviceSlug, environment)
	if err != nil {
		return page.Page[entities.Revision]{}, err
	}
	before, err := p.Number()
	if err != nil {
		return page.Page[entities.Revision]{}, err
	}
	list, err := u.store.History(ctx, t.service.ID, t.environment, before, p.Fetch())
	return page.Cut(list, p, revisionKey), err
}

func revisionKey(r entities.Revision) string {
	return strconv.Itoa(r.Number)
}

func (u ConfigUsecase) SecretsFor(ctx context.Context, a actor.Actor, serviceSlug, environment string) ([]entities.SecretInfo, error) {
	if err := u.visible(ctx, a, serviceSlug); err != nil {
		return nil, err
	}
	return u.Secrets(ctx, serviceSlug, environment)
}
