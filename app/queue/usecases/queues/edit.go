package queues

import (
	"context"
	"slices"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/uuid"
)

type Draft struct {
	Name        string
	Environment string
	Consumer    string
	Settings    entities.Settings
	Publishers  []string
	Message     string
}

type Change struct {
	Settings    *entities.Settings
	Publishers  *[]string
	Message     string
	BaseVersion int
}

var errStale = apperror.Conflict("the queue changed since you loaded it, reload and try again")

func (u QueuesUsecase) Create(ctx context.Context, a actor.Actor, d Draft) (entities.Queue, error) {
	consumer, err := u.checkDraft(ctx, d)
	if err != nil {
		return entities.Queue{}, err
	}
	if err := u.authz.Can(ctx, a, authz.EditQueue, authz.Resource{TeamID: consumer.OwnerTeamID, Environment: d.Environment}); err != nil {
		return entities.Queue{}, err
	}
	q, err := u.newQueue(ctx, d, consumer)
	if err != nil {
		return entities.Queue{}, err
	}
	if err := u.store.Create(ctx, q, versionOf(a, q, d.Message)); err != nil {
		return entities.Queue{}, err
	}
	u.publish(ctx, a, "queue.created", q, map[string]any{"version": q.Version})
	return q, nil
}

func (u QueuesUsecase) checkDraft(ctx context.Context, d Draft) (ServiceRef, error) {
	if err := entities.ValidateName(d.Name); err != nil {
		return ServiceRef{}, err
	}
	if err := u.environments.EnvironmentExists(ctx, d.Environment); err != nil {
		return ServiceRef{}, err
	}
	return u.services.FindService(ctx, d.Consumer)
}

func (u QueuesUsecase) newQueue(ctx context.Context, d Draft, consumer ServiceRef) (entities.Queue, error) {
	settings := withDefaults(d.Settings)
	if err := settings.Validate(); err != nil {
		return entities.Queue{}, err
	}
	publishers, err := u.publisherIDs(ctx, consumer, d.Publishers)
	now := clock.Now().UTC()
	return entities.Queue{
		ID: uuid.NewUUID(), Name: d.Name, Environment: d.Environment, TeamID: consumer.OwnerTeamID,
		ConsumerID: consumer.ID, Version: 1, Settings: settings, Publishers: publishers, CreatedAt: now, UpdatedAt: now,
	}, err
}

func withDefaults(s entities.Settings) entities.Settings {
	if s == (entities.Settings{Path: s.Path}) {
		return entities.DefaultSettings(s.Path)
	}
	return s
}

func (u QueuesUsecase) publisherIDs(ctx context.Context, consumer ServiceRef, slugs []string) ([]string, error) {
	ids := []string{consumer.ID}
	for _, slug := range slugs {
		s, err := u.services.FindService(ctx, slug)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(ids, s.ID) {
			ids = append(ids, s.ID)
		}
	}
	return ids, nil
}

func (u QueuesUsecase) Update(ctx context.Context, a actor.Actor, environment, name string, c Change) (entities.Queue, error) {
	q, err := u.editable(ctx, a, environment, name)
	if err != nil {
		return entities.Queue{}, err
	}
	next, err := u.apply(ctx, q, c)
	if err != nil {
		return entities.Queue{}, err
	}
	if err := u.store.InsertVersion(ctx, next, versionOf(a, next, c.Message)); err != nil {
		return entities.Queue{}, err
	}
	u.publish(ctx, a, "queue.updated", next, map[string]any{"version": next.Version, "message": c.Message})
	return next, nil
}

func (u QueuesUsecase) apply(ctx context.Context, q entities.Queue, c Change) (entities.Queue, error) {
	if c.BaseVersion != q.Version {
		return entities.Queue{}, errStale
	}
	if c.Settings != nil {
		q.Settings = *c.Settings
	}
	if err := q.Settings.Validate(); err != nil {
		return entities.Queue{}, err
	}
	q, err := u.withPublishers(ctx, q, c.Publishers)
	q.Version, q.UpdatedAt = q.Version+1, clock.Now().UTC()
	return q, err
}

func (u QueuesUsecase) withPublishers(ctx context.Context, q entities.Queue, slugs *[]string) (entities.Queue, error) {
	if slugs == nil {
		return q, nil
	}
	consumer, err := u.services.FindServiceByID(ctx, q.ConsumerID)
	if err != nil {
		return entities.Queue{}, err
	}
	q.Publishers, err = u.publisherIDs(ctx, consumer, *slugs)
	return q, err
}

func versionOf(a actor.Actor, q entities.Queue, message string) entities.Version {
	return entities.Version{QueueID: q.ID, Number: q.Version, Settings: q.Settings, AuthorID: a.UserID, Message: message, CreatedAt: q.UpdatedAt}
}
