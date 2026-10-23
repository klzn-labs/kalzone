package cron

import (
	"context"
	"encoding/json"

	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/app/cron/usecases/crons"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Crons interface {
	ApplyApproved(ctx context.Context, a actor.Actor, o crons.Proposal) (entities.Cron, error)
	Preview(ctx context.Context, o crons.Proposal) (approval.Preview, error)
}

type Executor struct {
	crons Crons
}

func NewExecutor(crons Crons) Executor {
	return Executor{crons: crons}
}

var results = map[string]string{
	approval.KindCronCreate: "cron created",
	approval.KindCronUpdate: "cron changed",
	approval.KindCronDelete: "cron deleted",
}

func (e Executor) Execute(ctx context.Context, a actor.Actor, payload []byte) (string, error) {
	var o crons.Proposal
	if err := json.Unmarshal(payload, &o); err != nil {
		return "", err
	}
	_, err := e.crons.ApplyApproved(ctx, a, o)
	return results[o.Kind], err
}

func (e Executor) Preview(ctx context.Context, payload []byte) (approval.Preview, error) {
	var o crons.Proposal
	if err := json.Unmarshal(payload, &o); err != nil {
		return approval.Preview{}, err
	}
	return e.crons.Preview(ctx, o)
}
