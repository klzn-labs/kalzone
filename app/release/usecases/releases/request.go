package releases

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

var (
	errDecommissioning = apperror.Conflict("this service is being deleted").WithHint("its builds are frozen until the teardown finishes")
	errSimulated       = apperror.Conflict("demo services are simulated and never build").WithHint("build a service of your own team")
	errCannotBuild     = apperror.Invalid("set the service's GitHub repository and image repository before building")
	errDispatchFails   = apperror.New(apperror.CodeUpstream, "GitHub did not start the build")
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

func buildable(service entities.ServiceRef) error {
	switch {
	case service.Simulated:
		return errSimulated
	case service.Decommissioning:
		return errDecommissioning
	}
	return nil
}

func (u ReleasesUsecase) build(ctx context.Context, a actor.Actor, service entities.ServiceRef, ref string, trigger entities.Trigger) (entities.Release, error) {
	if err := buildable(service); err != nil {
		return entities.Release{}, err
	}
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

func (u ReleasesUsecase) FailUnfinished(ctx context.Context, a actor.Actor, serviceID string) error {
	service, err := u.services.FindServiceByID(ctx, serviceID)
	if err != nil {
		return err
	}
	open, err := u.store.UnfinishedFor(ctx, serviceID)
	if err != nil {
		return err
	}
	for _, rel := range open {
		if _, err := u.fail(ctx, a, service, rel, "the service is being deleted"); err != nil {
			return err
		}
	}
	return nil
}
