package cron

import (
	"context"

	cronEntities "github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/app/resources/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Crons interface {
	ListFor(ctx context.Context, a actor.Actor, environment, teamSlug string) ([]cronEntities.Cron, error)
}

type ServiceNames interface {
	SlugOf(ctx context.Context, id string) (string, error)
}

type Source struct {
	crons Crons
	names ServiceNames
}

func NewSource(crons Crons, names ServiceNames) Source {
	return Source{crons: crons, names: names}
}

func (s Source) Resources(ctx context.Context, a actor.Actor, environment string) ([]entities.Resource, error) {
	list, err := s.crons.ListFor(ctx, a, environment, "")
	if err != nil {
		return nil, err
	}
	result := make([]entities.Resource, 0, len(list))
	for _, c := range list {
		result = append(result, s.resource(ctx, c))
	}
	return result, nil
}

func (s Source) resource(ctx context.Context, c cronEntities.Cron) entities.Resource {
	attached := []string{}
	if slug, err := s.names.SlugOf(ctx, c.ServiceID); err == nil {
		attached = append(attached, slug)
	}
	return entities.Resource{
		Kind: entities.KindCron, Name: c.Name, Environment: c.Environment, TeamID: c.TeamID,
		Attached: attached, CreatedAt: c.CreatedAt,
	}
}
