package deploy

import (
	"context"
	"slices"
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
)

type Deployments interface {
	LatestSucceeded(ctx context.Context, serviceID, environment string) (entities.Deployment, bool, error)
	Active(ctx context.Context, serviceID, environment string) ([]entities.Deployment, error)
}

type Running struct {
	deployments Deployments
}

func NewRunning(deployments Deployments) Running {
	return Running{deployments: deployments}
}

func (r Running) RunsSince(ctx context.Context, serviceID, environment string, since time.Time) (bool, error) {
	live, err := r.liveSince(ctx, serviceID, environment, since)
	if err != nil || live {
		return live, err
	}
	return r.startedSince(ctx, serviceID, environment, since)
}

func (r Running) liveSince(ctx context.Context, serviceID, environment string, since time.Time) (bool, error) {
	live, found, err := r.deployments.LatestSucceeded(ctx, serviceID, environment)
	if err != nil || !found || live.Strategy == entities.StrategyRemove {
		return false, err
	}
	return !startOf(live).After(since), nil
}

func (r Running) startedSince(ctx context.Context, serviceID, environment string, since time.Time) (bool, error) {
	active, err := r.deployments.Active(ctx, serviceID, environment)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(active, func(d entities.Deployment) bool { return claimedBy(d, since) }), nil
}

func claimedBy(d entities.Deployment, since time.Time) bool {
	return d.Status == entities.StatusInProgress && d.StartedAt != nil && !d.StartedAt.After(since)
}

func startOf(d entities.Deployment) time.Time {
	if d.StartedAt != nil {
		return *d.StartedAt
	}
	return d.CreatedAt
}
