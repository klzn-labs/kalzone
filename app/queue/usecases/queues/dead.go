package queues

import (
	"context"

	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/pkg/actor"
)

func (u QueuesUsecase) Replay(ctx context.Context, a actor.Actor, environment, name, messageID string) error {
	return u.deadCommand(ctx, a, environment, name, entities.CommandReplay, messageID, "queue.dead_replayed")
}

func (u QueuesUsecase) ReplayAll(ctx context.Context, a actor.Actor, environment, name string) error {
	return u.deadCommand(ctx, a, environment, name, entities.CommandReplayAll, "", "queue.dead_replayed_all")
}

func (u QueuesUsecase) PurgeDead(ctx context.Context, a actor.Actor, environment, name string) error {
	return u.deadCommand(ctx, a, environment, name, entities.CommandPurgeDead, "", "queue.dead_purged")
}

func (u QueuesUsecase) deadCommand(ctx context.Context, a actor.Actor, environment, name, kind, argument, action string) error {
	q, err := u.editable(ctx, a, environment, name)
	if err != nil {
		return err
	}
	if err := u.command(ctx, q, kind, argument); err != nil {
		return err
	}
	u.publish(ctx, a, action, q, map[string]any{"message": argument})
	return nil
}
