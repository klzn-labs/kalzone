package deployments

import (
	"context"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
)

var (
	errNothingLive     = apperror.Invalid("nothing is live there yet, deploy a release first")
	errConfigIsLive    = apperror.Invalid("the live deployment already runs the current config")
	errSameEnvironment = apperror.Invalid("promote between two different environments")
	errNotSucceeded    = apperror.Invalid("only a deployment that succeeded can be rolled back to")
)

type Request struct {
	Service     string
	Environment string
	ReleaseID   string
	Strategy    string
}

func (u DeploymentsUsecase) Deploy(ctx context.Context, a actor.Actor, r Request) (entities.Deployment, error) {
	strategy, err := entities.ParseStrategy(r.Strategy)
	if err != nil {
		return entities.Deployment{}, err
	}
	service, err := u.authorize(ctx, a, r.Service, r.Environment)
	if err != nil {
		return entities.Deployment{}, err
	}
	rel, err := u.deployable(ctx, service, r.ReleaseID)
	if err != nil {
		return entities.Deployment{}, err
	}
	config, err := u.configs.CurrentNumber(ctx, service.Slug, r.Environment)
	if err != nil {
		return entities.Deployment{}, err
	}
	return u.schedule(ctx, plan{a, service, r.Environment, rel, config, strategy, entities.TriggerManual})
}

func (u DeploymentsUsecase) ApplyConfig(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.Deployment, error) {
	service, err := u.authorize(ctx, a, serviceSlug, environment)
	if err != nil {
		return entities.Deployment{}, err
	}
	live, rel, err := u.live(ctx, service, environment)
	if err != nil {
		return entities.Deployment{}, err
	}
	config, err := u.configs.CurrentNumber(ctx, service.Slug, environment)
	if err != nil {
		return entities.Deployment{}, err
	}
	if config == live.ConfigRevision {
		return entities.Deployment{}, errConfigIsLive
	}
	return u.schedule(ctx, plan{a, service, environment, rel, config, live.Strategy, entities.TriggerConfig})
}

func (u DeploymentsUsecase) Promote(ctx context.Context, a actor.Actor, serviceSlug, from, to string) (entities.Deployment, error) {
	if from == to {
		return entities.Deployment{}, errSameEnvironment
	}
	service, err := u.authorize(ctx, a, serviceSlug, to)
	if err != nil {
		return entities.Deployment{}, err
	}
	_, rel, err := u.live(ctx, service, from)
	if err != nil {
		return entities.Deployment{}, err
	}
	config, err := u.configs.CurrentNumber(ctx, service.Slug, to)
	if err != nil {
		return entities.Deployment{}, err
	}
	return u.schedule(ctx, plan{a, service, to, rel, config, entities.StrategyBlueGreen, entities.TriggerPromote})
}

func (u DeploymentsUsecase) Rollback(ctx context.Context, a actor.Actor, deploymentID string) (entities.Deployment, error) {
	target, err := u.store.FindByID(ctx, deploymentID)
	if err != nil {
		return entities.Deployment{}, err
	}
	if target.Status != entities.StatusSucceeded {
		return entities.Deployment{}, errNotSucceeded
	}
	service, err := u.services.FindServiceByID(ctx, target.ServiceID)
	if err != nil {
		return entities.Deployment{}, err
	}
	if _, err := u.authorize(ctx, a, service.Slug, target.Environment); err != nil {
		return entities.Deployment{}, err
	}
	rel := entities.ReleaseRef{ID: target.ReleaseID, ServiceID: service.ID, Number: target.ReleaseNumber, Built: true, Reference: target.ImageRef}
	return u.schedule(ctx, plan{a, service, target.Environment, rel, target.ConfigRevision, entities.StrategyBlueGreen, entities.TriggerRollback})
}

func (u DeploymentsUsecase) live(ctx context.Context, service entities.ServiceRef, environment string) (entities.Deployment, entities.ReleaseRef, error) {
	live, found, err := u.store.LatestSucceeded(ctx, service.ID, environment)
	if err != nil {
		return entities.Deployment{}, entities.ReleaseRef{}, err
	}
	if !found {
		return entities.Deployment{}, entities.ReleaseRef{}, errNothingLive
	}
	rel := entities.ReleaseRef{ID: live.ReleaseID, ServiceID: service.ID, Number: live.ReleaseNumber, Built: true, Reference: live.ImageRef}
	return live, rel, nil
}
