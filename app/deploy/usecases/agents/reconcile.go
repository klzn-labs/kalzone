package agents

import (
	"context"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

func (u AgentsUsecase) Desired(ctx context.Context) ([]entities.Desired, error) {
	live, err := u.store.AllLive(ctx)
	if err != nil {
		return nil, err
	}
	desired := make([]entities.Desired, 0, len(live))
	for _, d := range live {
		if d.Strategy == entities.StrategyRemove {
			continue
		}
		service, err := u.services.FindServiceByID(ctx, d.ServiceID)
		if err != nil || service.Simulated {
			continue
		}
		desired = append(desired, entities.Desired{
			Name: entities.WorkloadName(service.Slug, d.Environment), Service: service.Slug,
			Environment: d.Environment, ImageRef: d.ImageRef, DeploymentID: d.ID,
		})
	}
	return desired, nil
}

func (u AgentsUsecase) Drift(ctx context.Context, agent string, drift entities.Drift) (entities.Deployment, bool, error) {
	service, err := u.services.FindService(ctx, drift.Service)
	if err != nil {
		return entities.Deployment{}, false, err
	}
	live, found, err := u.store.LatestSucceeded(ctx, service.ID, drift.Environment)
	if err != nil || !found {
		return entities.Deployment{}, false, err
	}
	active, err := u.store.Active(ctx, service.ID, drift.Environment)
	if err != nil || len(active) > 0 {
		return entities.Deployment{}, false, err
	}
	d, err := u.store.Create(ctx, reconcileOf(live))
	if err != nil {
		return entities.Deployment{}, false, err
	}
	u.publish(ctx, agentActor(agent), "deploy.drift", d, truncate(drift.Observed, 200))
	return d, true, nil
}

func reconcileOf(live entities.Deployment) entities.Deployment {
	return entities.Deployment{
		ID: uuid.NewUUID(), ServiceID: live.ServiceID, Environment: live.Environment, ReleaseID: live.ReleaseID,
		ReleaseNumber: live.ReleaseNumber, ImageRef: live.ImageRef, ConfigRevision: live.ConfigRevision,
		Strategy: entities.StrategyBlueGreen, Trigger: entities.TriggerReconcile, Status: entities.StatusPending,
		CreatedAt: clock.Now().UTC(),
	}
}

func (u AgentsUsecase) Sweep(ctx context.Context) (int, error) {
	stale, err := u.store.StaleInProgress(ctx, clock.Now().UTC().Add(-u.timeout))
	if err != nil {
		return 0, err
	}
	swept := 0
	for _, d := range stale {
		if _, err := u.finish(ctx, d.Agent, d, false, "the agent stopped reporting"); err == nil {
			swept++
		}
	}
	return swept, nil
}
