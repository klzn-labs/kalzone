package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Queues interface {
	ApplyApproved(ctx context.Context, a actor.Actor, o queues.Proposal) (entities.Queue, error)
	Preview(ctx context.Context, o queues.Proposal) (approval.Preview, error)
}

type Executor struct {
	queues Queues
}

func NewExecutor(queues Queues) Executor {
	return Executor{queues: queues}
}

func (e Executor) Execute(ctx context.Context, a actor.Actor, payload []byte) (string, error) {
	var o queues.Proposal
	if err := json.Unmarshal(payload, &o); err != nil {
		return "", err
	}
	q, err := e.queues.ApplyApproved(ctx, a, o)
	if o.Kind == approval.KindQueueDelete {
		return "queue deleted", err
	}
	return fmt.Sprintf("queue version %d", q.Version), err
}

func (e Executor) Preview(ctx context.Context, payload []byte) (approval.Preview, error) {
	var o queues.Proposal
	if err := json.Unmarshal(payload, &o); err != nil {
		return approval.Preview{}, err
	}
	return e.queues.Preview(ctx, o)
}
