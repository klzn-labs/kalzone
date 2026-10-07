package deployments

import (
	"context"
	"fmt"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/uuid"
)

var (
	errRunning      = apperror.Conflict("a deployment is already running here, wait for it to finish")
	errNotBuilt     = apperror.Invalid("only built releases can be deployed")
	errOtherService = apperror.Invalid("that release belongs to another service")
)

type plan struct {
	actor    actor.Actor
	service  entities.ServiceRef
	env      string
	release  entities.ReleaseRef
	config   int
	strategy entities.Strategy
	trigger  entities.Trigger
}

func (u DeploymentsUsecase) authorize(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.ServiceRef, error) {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return entities.ServiceRef{}, err
	}
	if err := u.environments.EnvironmentExists(ctx, environment); err != nil {
		return entities.ServiceRef{}, err
	}
	resource := authz.Resource{TeamID: service.OwnerTeamID, Environment: environment}
	return service, u.authz.Can(ctx, a, authz.Deploy, resource)
}

func (u DeploymentsUsecase) deployable(ctx context.Context, service entities.ServiceRef, releaseID string) (entities.ReleaseRef, error) {
	rel, err := u.releases.FindRelease(ctx, releaseID)
	if err != nil {
		return entities.ReleaseRef{}, err
	}
	if rel.ServiceID != service.ID {
		return entities.ReleaseRef{}, errOtherService
	}
	if !rel.Built {
		return entities.ReleaseRef{}, errNotBuilt
	}
	return rel, nil
}

func (u DeploymentsUsecase) schedule(ctx context.Context, p plan) (entities.Deployment, error) {
	if err := u.clearWay(ctx, p.service.ID, p.env); err != nil {
		return entities.Deployment{}, err
	}
	d, err := u.store.Create(ctx, entities.Deployment{
		ID: uuid.NewUUID(), ServiceID: p.service.ID, Environment: p.env, ReleaseID: p.release.ID,
		ReleaseNumber: p.release.Number, ImageRef: p.release.Reference, ConfigRevision: p.config,
		Strategy: p.strategy, Trigger: p.trigger, Status: entities.StatusPending, RequestedBy: p.actor.UserID,
		CreatedAt: clock.Now().UTC(),
	})
	if err != nil {
		return entities.Deployment{}, err
	}
	publish(ctx, u.publisher, p.actor, "deploy.requested", p.service, d)
	return d, nil
}

func (u DeploymentsUsecase) clearWay(ctx context.Context, serviceID, environment string) error {
	active, err := u.store.Active(ctx, serviceID, environment)
	if err != nil {
		return err
	}
	for _, d := range active {
		if d.Status == entities.StatusInProgress {
			return errRunning
		}
	}
	for _, d := range active {
		if err := u.cancel(ctx, d); err != nil {
			return err
		}
	}
	return nil
}

func (u DeploymentsUsecase) cancel(ctx context.Context, d entities.Deployment) error {
	now := clock.Now().UTC()
	d.Status, d.Message, d.FinishedAt = entities.StatusCancelled, fmt.Sprintf("superseded before deployment #%d started", d.Number), &now
	return u.store.Transition(ctx, d, entities.StatusPending)
}
