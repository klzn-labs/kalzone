package queues

import (
	"context"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/approval"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/uuid"
)

var errNotEmpty = apperror.Conflict("the queue still holds messages; empty it or delete with force")

func (u QueuesUsecase) SetPaused(ctx context.Context, a actor.Actor, environment, name string, paused bool) (entities.Queue, error) {
	q, gate, err := u.operable(ctx, a, environment, name)
	if err != nil {
		return entities.Queue{}, err
	}
	if err := u.store.SetPaused(ctx, q.ID, paused); err != nil {
		return entities.Queue{}, err
	}
	q.Paused = paused
	u.publish(ctx, a, gate, map[bool]string{true: "queue.paused", false: "queue.resumed"}[paused], q, nil)
	return q, nil
}

func (u QueuesUsecase) Delete(ctx context.Context, a actor.Actor, environment, name string, force bool) error {
	q, gate, err := u.editable(ctx, a, environment, name)
	if err != nil {
		return err
	}
	if !force && !u.knownEmpty(q.ID) {
		return errNotEmpty
	}
	if gate == authz.NeedsApproval {
		return u.propose(ctx, a, q, Proposal{Kind: approval.KindQueueDelete, Force: force})
	}
	if err := u.store.Delete(ctx, q.ID, newCommand(q, entities.CommandDelete, q.Environment+"/"+q.Name)); err != nil {
		return err
	}
	u.publish(ctx, a, gate, "queue.deleted", q, map[string]any{"forced": force})
	return nil
}

func (u QueuesUsecase) knownEmpty(queueID string) bool {
	st, ok := u.stats.Latest(queueID)
	return ok && st.Fresh(clock.Now().UTC()) && st.Depth == 0 && st.DeadLetters == 0
}

func (u QueuesUsecase) command(ctx context.Context, q entities.Queue, kind, argument string) error {
	return u.store.AddCommand(ctx, newCommand(q, kind, argument))
}

func newCommand(q entities.Queue, kind, argument string) entities.Command {
	return entities.Command{ID: uuid.NewUUID(), QueueID: q.ID, Kind: kind, Argument: argument, CreatedAt: clock.Now().UTC()}
}
