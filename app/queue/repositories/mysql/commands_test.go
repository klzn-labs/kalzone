package mysql_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/stretchr/testify/require"
)

func Test_Commands_PendingUntilFinished(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	require.NoError(t, repo.AddCommand(ctx, entities.Command{ID: "c1", QueueID: "q1", Kind: entities.CommandReplay, Argument: "m1", CreatedAt: at}))
	require.NoError(t, repo.AddCommand(ctx, entities.Command{ID: "c2", QueueID: "q1", Kind: entities.CommandPurgeDead, CreatedAt: at}))

	require.NoError(t, repo.FinishCommand(ctx, "c1", at))

	pending, err := repo.PendingCommands(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "c2", pending[0].ID)
}
