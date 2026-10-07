package releases

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/release/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/uuid"
)

var (
	errCannotBuild   = apperror.Invalid("set the service's GitHub repository and image repository before building")
	errDispatchFails = apperror.New(apperror.CodeUpstream, "GitHub did not start the build")
)

var GitHubActor = actor.Actor{Login: "github"}

func (u ReleasesUsecase) Request(ctx context.Context, a actor.Actor, serviceSlug, ref string) (entities.Release, error) {
	ref = strings.TrimSpace(ref)
	if err := entities.ValidateRef(ref); err != nil {
		return entities.Release{}, err
	}
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return entities.Release{}, err
	}
	if err := u.authz.Can(ctx, a, authz.CreateRelease, authz.Resource{TeamID: service.OwnerTeamID}); err != nil {
		return entities.Release{}, err
	}
	return u.build(ctx, a, service, ref, entities.TriggerManual)
}

func (u ReleasesUsecase) build(ctx context.Context, a actor.Actor, service entities.ServiceRef, ref string, trigger entities.Trigger) (entities.Release, error) {
	if service.GitHubRepo == "" || service.ImageRepository == "" {
		return entities.Release{}, errCannotBuild
	}
	rel, err := u.store.Create(ctx, entities.Release{
		ID: uuid.NewUUID(), ServiceID: service.ID, Source: entities.SourceBuild, GitRef: ref, Image: service.ImageRepository,
		Status: entities.StatusRequested, Trigger: trigger, RequestedBy: a.UserID, CreatedAt: clock.Now().UTC(),
	})
	if err != nil {
		return entities.Release{}, err
	}
	u.publish(ctx, a, "release.requested", service, rel)
	return u.dispatch(ctx, a, service, rel)
}

func (u ReleasesUsecase) dispatch(ctx context.Context, a actor.Actor, service entities.ServiceRef, rel entities.Release) (entities.Release, error) {
	err := u.builder.Dispatch(ctx, entities.BuildRequest{
		Repository: service.GitHubRepo, Workflow: service.BuildWorkflow, Ref: rel.GitRef,
		Inputs: map[string]string{"release_id": rel.ID, "image": rel.Image, "platform_url": u.platformURL},
	})
	if err != nil {
		failed, failErr := u.fail(ctx, a, service, rel, "dispatch failed: "+err.Error())
		return failed, errors.Join(fmt.Errorf("%w: %s", errDispatchFails, err.Error()), failErr)
	}
	building := rel
	building.Status = entities.StatusBuilding
	return building, u.store.Transition(ctx, building, entities.StatusRequested)
}

func (u ReleasesUsecase) fail(ctx context.Context, a actor.Actor, service entities.ServiceRef, rel entities.Release, reason string) (entities.Release, error) {
	from := rel.Status
	now := clock.Now().UTC()
	rel.Status, rel.Failure, rel.FinishedAt = entities.StatusFailed, truncate(reason, 500), &now
	if err := u.store.Transition(ctx, rel, from); err != nil {
		return entities.Release{}, err
	}
	u.publish(ctx, a, "release.failed", service, rel)
	return rel, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
