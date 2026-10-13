package releases

import (
	"context"
	"strconv"

	"github.com/adrianozp/platform/app/release/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/page"
)

var (
	errNotVisible        = apperror.NotFound("release not found").WithHint(listReleasesHint)
	errServiceNotVisible = apperror.NotFound("service not found").WithHint("list services with GET /api/v1/services")
	errServiceRequired   = apperror.Invalid("service is required").WithField("service").
				WithHint("send ?service=<slug>; list services with GET /api/v1/services")
)

const listReleasesHint = "list a service's releases with GET /api/v1/releases?service=<slug>"

func (u ReleasesUsecase) PageFor(ctx context.Context, a actor.Actor, serviceSlug string, p page.Request) (page.Page[entities.Release], error) {
	service, err := u.visibleService(ctx, a, serviceSlug)
	if err != nil {
		return page.Page[entities.Release]{}, err
	}
	before, err := p.Number()
	if err != nil {
		return page.Page[entities.Release]{}, err
	}
	list, err := u.store.ListByService(ctx, service.ID, before, p.Fetch())
	return page.Cut(list, p, releaseKey), err
}

func (u ReleasesUsecase) visibleService(ctx context.Context, a actor.Actor, serviceSlug string) (entities.ServiceRef, error) {
	if serviceSlug == "" {
		return entities.ServiceRef{}, errServiceRequired
	}
	service, err := u.services.FindService(ctx, serviceSlug)
	if err == nil && !a.Sees(service.OwnerTeamID) {
		return entities.ServiceRef{}, errServiceNotVisible
	}
	return service, err
}

func releaseKey(r entities.Release) string {
	return strconv.Itoa(r.Number)
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
