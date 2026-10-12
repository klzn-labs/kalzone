package deployments

import (
	"context"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/approval"
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
	p, err := u.authorize(ctx, a, r.Service, r.Environment)
	if err != nil {
		return entities.Deployment{}, err
	}
	if p.release, err = u.deployable(ctx, p.service, r.ReleaseID); err != nil {
		return entities.Deployment{}, err
	}
	if p.config, err = u.configs.CurrentNumber(ctx, p.service.Slug, p.env); err != nil {
		return entities.Deployment{}, err
	}
	p.strategy, p.trigger = strategy, entities.TriggerManual
	return u.proceed(ctx, p, Proposal{Kind: approval.KindDeploy, ReleaseID: p.release.ID, Strategy: string(strategy)})
}

func (u DeploymentsUsecase) ApplyConfig(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.Deployment, error) {
	p, err := u.authorize(ctx, a, serviceSlug, environment)
	if err != nil {
		return entities.Deployment{}, err
	}
	live, rel, err := u.live(ctx, p.service, environment)
	if err != nil {
		return entities.Deployment{}, err
	}
	config, err := u.configs.CurrentNumber(ctx, p.service.Slug, environment)
	if err != nil {
		return entities.Deployment{}, err
	}
	if config == live.ConfigRevision {
		return entities.Deployment{}, errConfigIsLive
	}
	p.release, p.config, p.strategy, p.trigger = rel, config, live.Strategy, entities.TriggerConfig
	return u.proceed(ctx, p, Proposal{Kind: approval.KindConfigApply})
}

func (u DeploymentsUsecase) Promote(ctx context.Context, a actor.Actor, serviceSlug, from, to string) (entities.Deployment, error) {
	if from == to {
		return entities.Deployment{}, errSameEnvironment
	}
	p, err := u.authorize(ctx, a, serviceSlug, to)
	if err != nil {
		return entities.Deployment{}, err
	}
	if _, p.release, err = u.live(ctx, p.service, from); err != nil {
		return entities.Deployment{}, err
	}
	return u.promote(ctx, p, from)
}

func (u DeploymentsUsecase) promote(ctx context.Context, p plan, from string) (entities.Deployment, error) {
	config, err := u.configs.CurrentNumber(ctx, p.service.Slug, p.env)
	if err != nil {
		return entities.Deployment{}, err
	}
	p.config, p.strategy, p.trigger = config, entities.StrategyBlueGreen, entities.TriggerPromote
	return u.proceed(ctx, p, Proposal{Kind: approval.KindPromote, ReleaseID: p.release.ID, From: from})
}

func (u DeploymentsUsecase) Rollback(ctx context.Context, a actor.Actor, deploymentID string) (entities.Deployment, error) {
	target, err := u.store.FindByID(ctx, deploymentID)
	if err != nil {
		return entities.Deployment{}, err
	}
	service, err := u.services.FindServiceByID(ctx, target.ServiceID)
	if err != nil {
		return entities.Deployment{}, err
	}
	p, err := u.authorize(ctx, a, service.Slug, target.Environment)
	if err != nil {
		return entities.Deployment{}, err
	}
	if target.Status != entities.StatusSucceeded {
		return entities.Deployment{}, errNotSucceeded
	}
	p.release = entities.ReleaseRef{ID: target.ReleaseID, ServiceID: service.ID, Number: target.ReleaseNumber, Built: true, Reference: target.ImageRef}
	p.config, p.strategy, p.trigger = target.ConfigRevision, entities.StrategyBlueGreen, entities.TriggerRollback
	return u.proceed(ctx, p, Proposal{Kind: approval.KindRollback, DeploymentID: target.ID})
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
