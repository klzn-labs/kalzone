package deployments

import (
	"context"
	"fmt"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

var (
	errRunning = apperror.Conflict("a deployment is already running here, wait for it to finish").
			WithHint("follow the running deployment with GET /api/v1/deployments?service=<slug>&env=<env>, then deploy again")
	errNotBuilt = apperror.Invalid("only built releases can be deployed").
			WithHint("wait until GET /api/v1/releases/<id> says built")
	errOtherService    = apperror.Invalid("that release belongs to another service")
	errDecommissioning = apperror.Conflict("this service is being deleted").
				WithHint("its deployments are frozen until the teardown finishes")
)

type plan struct {
	actor    actor.Actor
	gate     authz.Gate
	service  entities.ServiceRef
	env      string
	release  entities.ReleaseRef
	config   int
	strategy entities.Strategy
	trigger  entities.Trigger
}

func (p plan) resource() authz.Resource {
	return authz.Resource{TeamID: p.service.OwnerTeamID, Environment: p.env}
}

func (u DeploymentsUsecase) authorize(ctx context.Context, a actor.Actor, serviceSlug, environment string) (plan, error) {
	p, err := u.authorizeAny(ctx, a, serviceSlug, environment)
	if err == nil && p.service.Decommissioning {
		return plan{}, errDecommissioning
	}
	return p, err
}

func (u DeploymentsUsecase) authorizeAny(ctx context.Context, a actor.Actor, serviceSlug, environment string) (plan, error) {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return plan{}, err
	}
	if err := u.environments.EnvironmentExists(ctx, environment); err != nil {
		return plan{}, err
	}
	p := plan{actor: a, service: service, env: environment}
	p.gate, err = u.authz.Gate(ctx, a, authz.Deploy, p.resource())
	return p, err
}

func (u DeploymentsUsecase) deployable(ctx context.Context, service entities.ServiceRef, ref string) (entities.ReleaseRef, error) {
	rel, err := u.releases.ResolveRelease(ctx, service.Slug, ref)
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

func (u DeploymentsUsecase) proceed(ctx context.Context, p plan, o Proposal) (entities.Deployment, error) {
	if p.gate == authz.NeedsApproval {
		return entities.Deployment{}, u.propose(ctx, p, o)
	}
	return u.schedule(ctx, p)
}

func (u DeploymentsUsecase) schedule(ctx context.Context, p plan) (entities.Deployment, error) {
	if err := u.clearWay(ctx, p.service.ID, p.env); err != nil {
		return entities.Deployment{}, err
	}
	d, err := u.store.Create(ctx, newDeployment(p))
	if err != nil {
		return entities.Deployment{}, err
	}
	u.announce(ctx, p, d)
	return d, nil
}

func newDeployment(p plan) entities.Deployment {
	return entities.Deployment{
		ID: uuid.NewUUID(), ServiceID: p.service.ID, Environment: p.env, ReleaseID: p.release.ID,
		ReleaseNumber: p.release.Number, ImageRef: p.release.Reference, ConfigRevision: p.config,
		Strategy: p.strategy, Trigger: p.trigger, Status: entities.StatusPending, RequestedBy: p.actor.UserID,
		CreatedAt: clock.Now().UTC(),
	}
}

func (u DeploymentsUsecase) announce(ctx context.Context, p plan, d entities.Deployment) {
	publish(ctx, u.publisher, p.actor, "deploy.requested", p.service, d)
	authz.PublishBypass(ctx, u.publisher, p.gate, p.actor, authz.Deploy, p.resource())
}

func (u DeploymentsUsecase) idle(ctx context.Context, serviceID, environment string) ([]entities.Deployment, error) {
	active, err := u.store.Active(ctx, serviceID, environment)
	if err != nil {
		return nil, err
	}
	for _, d := range active {
		if d.Status == entities.StatusInProgress {
			return nil, errRunning
		}
	}
	return active, nil
}

func (u DeploymentsUsecase) clearWay(ctx context.Context, serviceID, environment string) error {
	active, err := u.idle(ctx, serviceID, environment)
	if err != nil {
		return err
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
