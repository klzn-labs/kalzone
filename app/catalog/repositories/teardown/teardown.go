package teardown

import (
	"context"
	"errors"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Deployments interface {
	Remove(ctx context.Context, a actor.Actor, serviceSlug, environment string) (deploy.Deployment, error)
	Freeze(ctx context.Context, serviceID string) error
	Teardown(ctx context.Context, serviceID string) (map[string]bool, error)
	Removing(ctx context.Context, serviceID, environment string) (bool, error)
	ForgetRuntime(ctx context.Context, serviceID string) error
}

type Host struct {
	Deployments
}

func NewHost(d Deployments) Host {
	return Host{Deployments: d}
}

func (h Host) Remove(ctx context.Context, a actor.Actor, serviceSlug, environment string) (catalog.TeardownStep, error) {
	d, err := h.Deployments.Remove(ctx, a, serviceSlug, environment)
	var pending *approval.Pending
	if errors.As(err, &pending) {
		return catalog.TeardownStep{
			Environment: environment, Status: catalog.TeardownAwaitingApproval, RequestID: pending.ID, ApproveURL: pending.ApproveURL,
		}, nil
	}
	return catalog.TeardownStep{Environment: environment, Status: catalog.TeardownRemoving, DeploymentID: d.ID}, err
}
