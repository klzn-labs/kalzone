package deploy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/app/deploy/usecases/deployments"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/approval"
)

type Deployments interface {
	ApplyApproved(ctx context.Context, a actor.Actor, o deployments.Proposal) (entities.Deployment, error)
	Preview(ctx context.Context, o deployments.Proposal) (approval.Preview, error)
}

type Executor struct {
	deployments Deployments
}

func NewExecutor(deployments Deployments) Executor {
	return Executor{deployments: deployments}
}

func (e Executor) Execute(ctx context.Context, a actor.Actor, payload []byte) (string, error) {
	var o deployments.Proposal
	if err := json.Unmarshal(payload, &o); err != nil {
		return "", err
	}
	d, err := e.deployments.ApplyApproved(ctx, a, o)
	return fmt.Sprintf("deployment #%d (%s)", d.Number, d.ID), err
}

func (e Executor) Preview(ctx context.Context, payload []byte) (approval.Preview, error) {
	var o deployments.Proposal
	if err := json.Unmarshal(payload, &o); err != nil {
		return approval.Preview{}, err
	}
	return e.deployments.Preview(ctx, o)
}
