package deployments

import (
	"context"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/approval"
)

func (u DeploymentsUsecase) Remove(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.Deployment, error) {
	p, err := u.removalPlan(ctx, a, serviceSlug, environment)
	if err != nil {
		return entities.Deployment{}, err
	}
	return u.proceed(ctx, p, Proposal{Kind: approval.KindRemove})
}

func (u DeploymentsUsecase) removalPlan(ctx context.Context, a actor.Actor, serviceSlug, environment string) (plan, error) {
	p, err := u.authorizeAny(ctx, a, serviceSlug, environment)
	if err != nil {
		return plan{}, err
	}
	live, _, err := u.store.LatestSucceeded(ctx, p.service.ID, environment)
	p.release = entities.ReleaseRef{ID: live.ReleaseID, ServiceID: p.service.ID, Number: live.ReleaseNumber}
	p.config, p.strategy, p.trigger = live.ConfigRevision, entities.StrategyRemove, entities.TriggerDecommission
	return p, err
}

func (u DeploymentsUsecase) Teardown(ctx context.Context, serviceID string) (map[string]bool, error) {
	environments, err := u.store.Environments(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	removed := make(map[string]bool, len(environments))
	for _, env := range environments {
		live, found, err := u.store.LatestSucceeded(ctx, serviceID, env)
		if err != nil {
			return nil, err
		}
		removed[env] = found && live.Strategy == entities.StrategyRemove
	}
	return removed, nil
}

func (u DeploymentsUsecase) Removing(ctx context.Context, serviceID, environment string) (bool, error) {
	active, err := u.store.Active(ctx, serviceID, environment)
	for _, d := range active {
		if d.Strategy == entities.StrategyRemove {
			return true, err
		}
	}
	return false, err
}

func (u DeploymentsUsecase) Freeze(ctx context.Context, serviceID string) error {
	environments, err := u.store.Environments(ctx, serviceID)
	if err != nil {
		return err
	}
	for _, env := range environments {
		if err := u.cancelChanges(ctx, serviceID, env); err != nil {
			return err
		}
	}
	return nil
}

func (u DeploymentsUsecase) cancelChanges(ctx context.Context, serviceID, environment string) error {
	active, err := u.idle(ctx, serviceID, environment)
	if err != nil {
		return err
	}
	for _, d := range active {
		if d.Strategy == entities.StrategyRemove {
			continue
		}
		if err := u.cancel(ctx, d); err != nil {
			return err
		}
	}
	return nil
}

func (u DeploymentsUsecase) ForgetRuntime(ctx context.Context, serviceID string) error {
	return u.store.DeleteRuntime(ctx, serviceID)
}
