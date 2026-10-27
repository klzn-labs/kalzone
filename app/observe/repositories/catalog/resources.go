package catalog

import (
	"context"

	database "github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/app/observe/entities"
	queue "github.com/klzn-labs/kalzone/app/queue/entities"
)

type DatabaseLister interface {
	All(ctx context.Context) ([]database.Database, error)
}

type QueueLister interface {
	List(ctx context.Context, environment, teamSlug string) ([]queue.Queue, error)
}

type Resources struct {
	databases DatabaseLister
	queues    QueueLister
}

func NewResources(databases DatabaseLister, queues QueueLister) Resources {
	return Resources{databases: databases, queues: queues}
}

func (r Resources) Resources(ctx context.Context) (entities.Resources, error) {
	dbs, err := r.databases.All(ctx)
	if err != nil {
		return entities.Resources{}, err
	}
	queues, err := r.queues.List(ctx, "", "")
	return entities.Resources{Databases: ownedDatabases(dbs), Queues: ownedQueues(queues)}, err
}

func ownedDatabases(dbs []database.Database) []entities.OwnedDatabase {
	result := make([]entities.OwnedDatabase, 0, len(dbs))
	for _, db := range dbs {
		owned := entities.Database{Environment: db.Environment, Schema: db.Schema, User: db.Username, MaxConnections: db.MaxConnections}
		result = append(result, entities.OwnedDatabase{TeamID: db.TeamID, Database: owned})
	}
	return result
}

func ownedQueues(queues []queue.Queue) []entities.OwnedQueue {
	result := make([]entities.OwnedQueue, 0, len(queues))
	for _, q := range queues {
		result = append(result, entities.OwnedQueue{TeamID: q.TeamID, ID: q.ID})
	}
	return result
}
