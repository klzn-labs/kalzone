package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/app/queue/repositories/mysql"
	"github.com/adrianozp/platform/internal/testdb"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

var at = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

func queue(id, env, name string) (entities.Queue, entities.Version) {
	settings := entities.DefaultSettings("/hooks/" + name)
	q := entities.Queue{
		ID: id, Name: name, Environment: env, TeamID: "team", ConsumerID: "consumer", Version: 1,
		Settings: settings, Publishers: []string{"consumer", "importer"}, CreatedAt: at, UpdatedAt: at,
	}
	return q, entities.Version{QueueID: id, Number: 1, Settings: settings, AuthorID: "ana", CreatedAt: at}
}

func Test_Create_GetReturnsSettingsAndPublishers(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	q, v := queue("q1", "prod", "place-import")
	require.NoError(t, repo.Create(ctx, q, v))

	got, err := repo.Get(ctx, "prod", "place-import")

	require.NoError(t, err)
	require.Equal(t, q.Settings, got.Settings)
	require.ElementsMatch(t, []string{"consumer", "importer"}, got.Publishers)
	require.Equal(t, 1, got.Version)
}

func Test_Get_MissingIsNotFound(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))

	_, err := repo.Get(ctx, "prod", "nope")

	require.ErrorIs(t, err, apperror.NotFound(""))
}

func Test_Create_SameNameInAnotherEnvironmentIsAllowed(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	prod, pv := queue("q1", "prod", "place-import")
	dev, dv := queue("q2", "dev", "place-import")
	again, av := queue("q3", "prod", "place-import")

	require.NoError(t, repo.Create(ctx, prod, pv))
	require.NoError(t, repo.Create(ctx, dev, dv))
	require.ErrorIs(t, repo.Create(ctx, again, av), apperror.Conflict(""))
}

func Test_InsertVersion_UpdatesTheQueueAndKeepsHistory(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	q, v := queue("q1", "prod", "place-import")
	require.NoError(t, repo.Create(ctx, q, v))
	q.Version, q.Settings.MaxAttempts, q.Publishers = 2, 9, []string{"consumer"}

	require.NoError(t, repo.InsertVersion(ctx, q, entities.Version{QueueID: "q1", Number: 2, Settings: q.Settings, AuthorID: "ana", CreatedAt: at}))

	got, _ := repo.Get(ctx, "prod", "place-import")
	history, err := repo.Versions(ctx, "q1", 10)
	require.NoError(t, err)
	require.Equal(t, 9, got.Settings.MaxAttempts)
	require.Equal(t, []string{"consumer"}, got.Publishers)
	require.Len(t, history, 2)
	require.Equal(t, 2, history[0].Number)
}

func Test_InsertVersion_SameNumberTwiceIsAConflict(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	q, v := queue("q1", "prod", "place-import")
	require.NoError(t, repo.Create(ctx, q, v))
	q.Version = 2
	next := entities.Version{QueueID: "q1", Number: 2, Settings: q.Settings, AuthorID: "ana", CreatedAt: at}
	require.NoError(t, repo.InsertVersion(ctx, q, next))

	err := repo.InsertVersion(ctx, q, next)

	require.ErrorIs(t, err, apperror.Conflict(""))
}

func Test_List_FiltersByEnvironmentAndTeam(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	a, av := queue("q1", "prod", "a")
	b, bv := queue("q2", "dev", "b")
	c, cv := queue("q3", "prod", "c")
	c.TeamID = "other"
	for _, pair := range []struct {
		q entities.Queue
		v entities.Version
	}{{a, av}, {b, bv}, {c, cv}} {
		require.NoError(t, repo.Create(ctx, pair.q, pair.v))
	}

	prod, _ := repo.List(ctx, "prod", "")
	team, _ := repo.List(ctx, "", "team")
	all, err := repo.All(ctx)

	require.NoError(t, err)
	require.Len(t, prod, 2)
	require.Len(t, team, 2)
	require.Len(t, all, 3)
}

func Test_Delete_RemovesTheQueueAndLeavesOnlyTheDeleteCommand(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	q, v := queue("q1", "prod", "place-import")
	require.NoError(t, repo.Create(ctx, q, v))
	require.NoError(t, repo.AddCommand(ctx, entities.Command{ID: "c1", QueueID: "q1", Kind: entities.CommandReplayAll, CreatedAt: at}))

	require.NoError(t, repo.Delete(ctx, "q1", entities.Command{ID: "c2", QueueID: "q1", Kind: entities.CommandDelete, CreatedAt: at}))

	_, err := repo.Get(ctx, "prod", "place-import")
	pending, _ := repo.PendingCommands(ctx)
	versions, _ := repo.Versions(ctx, "q1", 10)
	require.ErrorIs(t, err, apperror.NotFound(""))
	require.Len(t, pending, 1)
	require.Equal(t, "c2", pending[0].ID)
	require.Empty(t, versions)
}

func Test_SetPaused(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	q, v := queue("q1", "prod", "place-import")
	require.NoError(t, repo.Create(ctx, q, v))

	require.NoError(t, repo.SetPaused(ctx, "q1", true))

	got, _ := repo.Get(ctx, "prod", "place-import")
	require.True(t, got.Paused)
}
