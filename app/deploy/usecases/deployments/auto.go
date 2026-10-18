package deployments

import (
	"context"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

const AutoDeployEnvironment = "dev"

var AutoDeployActor = actor.Actor{Login: "auto-deploy"}

func (u DeploymentsUsecase) AutoDeploy(ctx context.Context, serviceID, releaseID string) (entities.Deployment, error) {
	p, err := u.autoDeployPlan(ctx, serviceID, releaseID)
	if err != nil {
		return entities.Deployment{}, err
	}
	return u.schedule(ctx, p)
}

func (u DeploymentsUsecase) autoDeployPlan(ctx context.Context, serviceID, releaseID string) (plan, error) {
	service, err := u.services.FindServiceByID(ctx, serviceID)
	if err != nil {
		return plan{}, err
	}
	if service.Decommissioning {
		return plan{}, errDecommissioning
	}
	p := plan{
		actor: AutoDeployActor, gate: authz.Open, service: service, env: AutoDeployEnvironment,
		strategy: entities.StrategyBlueGreen, trigger: entities.TriggerPush,
	}
	if p.release, err = u.deployable(ctx, service, releaseID); err != nil {
		return plan{}, err
	}
	p.config, err = u.configs.CurrentNumber(ctx, service.Slug, p.env)
	return p, err
}
