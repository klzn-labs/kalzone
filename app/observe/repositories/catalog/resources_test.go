package catalog_test

import (
	"context"
	"testing"

	database "github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/observe/repositories/catalog"
	queue "github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/stretchr/testify/require"
)

type allDatabases []database.Database

func (d allDatabases) All(context.Context) ([]database.Database, error) { return d, nil }

type allQueues []queue.Queue

func (q allQueues) List(_ context.Context, environment, team string) ([]queue.Queue, error) {
	if environment != "" || team != "" {
		return nil, nil
	}
	return q, nil
}

func Test_Resources_MapsEveryDatabaseAndQueueWithItsTeam(t *testing.T) {
	dbs := allDatabases{{TeamID: "t1", Environment: "dev", Schema: "ledger_dev", Username: "kz_ledger_dev", MaxConnections: 10}}
	queues := allQueues{{ID: "q1", TeamID: "t2"}}

	got, err := catalog.NewResources(dbs, queues).Resources(context.Background())

	require.NoError(t, err)
	require.Equal(t, entities.Resources{
		Databases: []entities.OwnedDatabase{{TeamID: "t1", Database: entities.Database{Environment: "dev", Schema: "ledger_dev", User: "kz_ledger_dev", MaxConnections: 10}}},
		Queues:    []entities.OwnedQueue{{TeamID: "t2", ID: "q1"}},
	}, got)
}
