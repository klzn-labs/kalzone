package database

import (
	"context"

	databaseEntities "github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/klzn-labs/kalzone/app/resources/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Databases interface {
	ListFor(ctx context.Context, a actor.Actor, environment, teamSlug string) ([]databaseEntities.Database, error)
	AttachmentsFor(ctx context.Context, a actor.Actor, environment, name string) ([]databases.AttachmentView, error)
}

type Source struct {
	databases Databases
}

func NewSource(databases Databases) Source {
	return Source{databases: databases}
}

func (s Source) Resources(ctx context.Context, a actor.Actor, environment string) ([]entities.Resource, error) {
	list, err := s.databases.ListFor(ctx, a, environment, "")
	if err != nil {
		return nil, err
	}
	result := make([]entities.Resource, 0, len(list))
	for _, db := range list {
		r, err := s.resource(ctx, a, db)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, nil
}

func (s Source) resource(ctx context.Context, a actor.Actor, db databaseEntities.Database) (entities.Resource, error) {
	views, err := s.databases.AttachmentsFor(ctx, a, db.Environment, db.Name)
	return entities.Resource{
		Kind: entities.KindDatabase, Name: db.Name, Environment: db.Environment, TeamID: db.TeamID,
		Attached: activeServices(views), CreatedAt: db.CreatedAt,
	}, err
}

func activeServices(views []databases.AttachmentView) []string {
	result := make([]string, 0, len(views))
	for _, v := range views {
		if v.DetachedAt == nil {
			result = append(result, v.Service)
		}
	}
	return result
}
