package envconfig

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/klzn-labs/kalzone/app/envconfig/usecases/configs"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Configs interface {
	ApplyApproved(ctx context.Context, a actor.Actor, p configs.Proposal) (entities.Revision, error)
	Preview(ctx context.Context, p configs.Proposal) (approval.Preview, error)
}

type Executor struct {
	configs Configs
}

func NewExecutor(configs Configs) Executor {
	return Executor{configs: configs}
}

func (e Executor) Execute(ctx context.Context, a actor.Actor, payload []byte) (string, error) {
	var p configs.Proposal
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", err
	}
	rev, err := e.configs.ApplyApproved(ctx, a, p)
	return fmt.Sprintf("config revision #%d", rev.Number), err
}

func (e Executor) Preview(ctx context.Context, payload []byte) (approval.Preview, error) {
	var p configs.Proposal
	if err := json.Unmarshal(payload, &p); err != nil {
		return approval.Preview{}, err
	}
	return e.configs.Preview(ctx, p)
}
