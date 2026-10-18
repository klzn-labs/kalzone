package queues

import (
	"context"
	"slices"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

const publisherRemoved = "publisher removed: the service was deleted"

func (u QueuesUsecase) Detach(ctx context.Context, a actor.Actor, serviceID string) error {
	all, err := u.store.List(ctx, "", "")
	if err != nil {
		return err
	}
	for _, q := range all {
		if err := u.detachFrom(ctx, a, q, serviceID); err != nil {
			return err
		}
	}
	return u.store.DeleteCredentials(ctx, serviceID)
}

func (u QueuesUsecase) detachFrom(ctx context.Context, a actor.Actor, q entities.Queue, serviceID string) error {
	if q.ConsumerID == serviceID {
		return u.dropQueue(ctx, a, q)
	}
	if !slices.Contains(q.Publishers, serviceID) {
		return nil
	}
	return u.dropPublisher(ctx, a, q, serviceID)
}

func (u QueuesUsecase) dropQueue(ctx context.Context, a actor.Actor, q entities.Queue) error {
	if err := u.store.Delete(ctx, q.ID, newCommand(q, entities.CommandDelete, q.Environment+"/"+q.Name)); err != nil {
		return err
	}
	u.publish(ctx, a, authz.Open, "queue.deleted", q, map[string]any{"forced": true, "reason": "its consumer was deleted"})
	return nil
}

func (u QueuesUsecase) dropPublisher(ctx context.Context, a actor.Actor, q entities.Queue, serviceID string) error {
	q.Publishers = slices.DeleteFunc(slices.Clone(q.Publishers), func(id string) bool { return id == serviceID })
	q.Version, q.UpdatedAt = q.Version+1, clock.Now().UTC()
	if err := u.store.InsertVersion(ctx, q, versionOf(a, q, publisherRemoved)); err != nil {
		return err
	}
	u.publish(ctx, a, authz.Open, "queue.updated", q, map[string]any{"version": q.Version, "message": publisherRemoved})
	return nil
}
