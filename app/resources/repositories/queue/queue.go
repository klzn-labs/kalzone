package queue

import (
	"context"

	queueEntities "github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/resources/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Queues interface {
	ListFor(ctx context.Context, a actor.Actor, environment, teamSlug string) ([]queueEntities.Queue, error)
}

type ServiceNames interface {
	SlugOf(ctx context.Context, id string) (string, error)
}

type Source struct {
	queues Queues
	names  ServiceNames
}

func NewSource(queues Queues, names ServiceNames) Source {
	return Source{queues: queues, names: names}
}

func (s Source) Resources(ctx context.Context, a actor.Actor, environment string) ([]entities.Resource, error) {
	list, err := s.queues.ListFor(ctx, a, environment, "")
	if err != nil {
		return nil, err
	}
	result := make([]entities.Resource, 0, len(list))
	for _, q := range list {
		result = append(result, s.resource(ctx, q))
	}
	return result, nil
}

func (s Source) resource(ctx context.Context, q queueEntities.Queue) entities.Resource {
	return entities.Resource{
		Kind: entities.KindQueue, Name: q.Name, Environment: q.Environment, TeamID: q.TeamID,
		Attached: s.slugs(ctx, q.Attached()), CreatedAt: q.CreatedAt,
	}
}

func (s Source) slugs(ctx context.Context, ids []string) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if slug, err := s.names.SlugOf(ctx, id); err == nil {
			result = append(result, slug)
		}
	}
	return result
}
