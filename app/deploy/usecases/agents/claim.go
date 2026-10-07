package agents

import (
	"context"
	"errors"
	"log"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
)

var (
	errNotYours   = apperror.Forbidden("this deployment is not running on this agent")
	errNotRunning = apperror.Conflict("this deployment is not running")
)

func (u AgentsUsecase) Heartbeat(ctx context.Context, name, version string) error {
	return u.store.TouchAgent(ctx, entities.Agent{Name: name, Version: version, LastSeenAt: clock.Now().UTC()})
}

func (u AgentsUsecase) Claim(ctx context.Context, agent string) (entities.Workload, bool, error) {
	for attempt := 0; attempt < claimAttempts; attempt++ {
		d, found, err := u.store.OldestPending(ctx)
		if err != nil || !found {
			return entities.Workload{}, false, err
		}
		w, taken, err := u.take(ctx, agent, d)
		if err != nil || taken {
			return w, taken, err
		}
	}
	return entities.Workload{}, false, nil
}

func (u AgentsUsecase) take(ctx context.Context, agent string, d entities.Deployment) (entities.Workload, bool, error) {
	now := clock.Now().UTC()
	d.Status, d.Agent, d.StartedAt = entities.StatusInProgress, agent, &now
	err := u.store.Transition(ctx, d, entities.StatusPending)
	if errors.Is(err, apperror.Conflict("")) {
		return entities.Workload{}, false, nil
	}
	if err != nil {
		return entities.Workload{}, false, err
	}
	w, err := u.workload(ctx, d)
	if err != nil {
		u.failClaimed(ctx, agent, d, "could not prepare the workload: "+err.Error())
		return entities.Workload{}, false, nil
	}
	return w, true, nil
}

func (u AgentsUsecase) workload(ctx context.Context, d entities.Deployment) (entities.Workload, error) {
	service, err := u.services.FindServiceByID(ctx, d.ServiceID)
	if err != nil {
		return entities.Workload{}, err
	}
	resolved, err := u.configs.ResolveConfig(ctx, d.ServiceID, d.Environment, d.ConfigRevision)
	return entities.Workload{
		Deployment: d, Name: entities.WorkloadName(service.Slug, d.Environment), Service: service.Slug,
		Version: entities.VersionLabel(d), Config: resolved,
	}, err
}

func (u AgentsUsecase) failClaimed(ctx context.Context, agent string, d entities.Deployment, reason string) {
	if _, err := u.finish(ctx, agent, d, false, reason); err != nil {
		log.Printf("deployment %s could not be failed: %v", d.ID, err)
	}
}
