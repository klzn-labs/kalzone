package releases

import (
	"context"
	"strings"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/release/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/uuid"
)

var errNoImage = apperror.Invalid("set the service's image repository before registering a tag")

func (u ReleasesUsecase) Register(ctx context.Context, a actor.Actor, serviceSlug, tag string) (entities.Release, error) {
	tag = strings.TrimSpace(tag)
	if err := entities.ValidateTag(tag); err != nil {
		return entities.Release{}, err
	}
	service, err := u.registrable(ctx, a, serviceSlug)
	if err != nil {
		return entities.Release{}, err
	}
	digest, err := u.resolver.Resolve(ctx, service.ImageRepository, tag)
	if err != nil {
		return entities.Release{}, err
	}
	rel, err := u.store.Create(ctx, registered(a, service, tag, digest))
	if err == nil {
		u.publish(ctx, a, "release.registered", service, rel)
	}
	return rel, err
}

func registered(a actor.Actor, service entities.ServiceRef, tag, digest string) entities.Release {
	now := clock.Now().UTC()
	return entities.Release{
		ID: uuid.NewUUID(), ServiceID: service.ID, Source: entities.SourceRegistered, Image: service.ImageRepository,
		Tag: tag, Digest: digest, Status: entities.StatusBuilt, Trigger: entities.TriggerRegister,
		RequestedBy: a.UserID, CreatedAt: now, FinishedAt: &now,
	}
}

func (u ReleasesUsecase) registrable(ctx context.Context, a actor.Actor, serviceSlug string) (entities.ServiceRef, error) {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return entities.ServiceRef{}, err
	}
	if err := u.authz.Can(ctx, a, authz.CreateRelease, authz.Resource{TeamID: service.OwnerTeamID}); err != nil {
		return entities.ServiceRef{}, err
	}
	if service.Decommissioning {
		return entities.ServiceRef{}, errDecommissioning
	}
	if service.ImageRepository == "" {
		return entities.ServiceRef{}, errNoImage
	}
	return service, nil
}
