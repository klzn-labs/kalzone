package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/app/cron/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

var at = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func cron(id, env, name string) entities.Cron {
	return entities.Cron{
		ID: id, Name: name, Environment: env, TeamID: "team", ServiceID: "svc", Schedule: "*/5 * * * *",
		Path: "/hooks/" + name, Payload: []byte(`{"full":true}`), Timeout: 90 * time.Second, CreatedAt: at, UpdatedAt: at,
	}
}

func Test_Create_GetRoundTrips(t *testing.T) {
	repo := mysql.NewCronRepository(testdb.Open(t))
	c := cron("c1", "dev", "nightly")
	require.NoError(t, repo.Create(ctx, c))

	got, err := repo.Get(ctx, "dev", "nightly")
	byID, _ := repo.GetByID(ctx, "c1")

	require.NoError(t, err)
	require.Equal(t, c, got)
	require.Equal(t, c, byID)
}

func Test_Create_KeepsAnEmptyPayloadEmpty(t *testing.T) {
	repo := mysql.NewCronRepository(testdb.Open(t))
	c := cron("c1", "dev", "nightly")
	c.Payload = nil
	require.NoError(t, repo.Create(ctx, c))

	got, err := repo.Get(ctx, "dev", "nightly")

	require.NoError(t, err)
	require.Empty(t, got.Payload)
}

func Test_Get_MissingIsNotFound(t *testing.T) {
	repo := mysql.NewCronRepository(testdb.Open(t))

	_, err := repo.Get(ctx, "dev", "nope")

	require.ErrorIs(t, err, apperror.NotFound(""))
}

func Test_Create_DuplicateIsConflict(t *testing.T) {
	repo := mysql.NewCronRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, cron("c1", "dev", "nightly")))

	err := repo.Create(ctx, cron("c2", "dev", "nightly"))

	require.ErrorIs(t, err, apperror.Conflict(""))
}

func Test_SaveAndSetPaused_ChangeTheRow(t *testing.T) {
	repo := mysql.NewCronRepository(testdb.Open(t))
	c := cron("c1", "dev", "nightly")
	require.NoError(t, repo.Create(ctx, c))
	c.Schedule, c.Timeout, c.UpdatedAt = "@hourly", time.Second, at.Add(time.Hour)

	require.NoError(t, repo.Save(ctx, c))
	require.NoError(t, repo.SetPaused(ctx, "c1", true))

	got, _ := repo.GetByID(ctx, "c1")
	c.Paused = true
	require.Equal(t, c, got)
}

func Test_List_FiltersByEnvironmentAndTeam(t *testing.T) {
	repo := mysql.NewCronRepository(testdb.Open(t))
	other := cron("c2", "prod", "billing")
	other.TeamID = "other"
	require.NoError(t, repo.Create(ctx, cron("c1", "dev", "nightly")))
	require.NoError(t, repo.Create(ctx, other))

	all, err := repo.All(ctx)
	require.NoError(t, err)
	byEnv, _ := repo.List(ctx, "prod", "")
	byTeam, _ := repo.List(ctx, "", "team")

	require.Len(t, all, 2)
	require.Len(t, byEnv, 1)
	require.Equal(t, "c1", byTeam[0].ID)
}

func Test_ForService_ListsOnlyThatServicesCrons(t *testing.T) {
	repo := mysql.NewCronRepository(testdb.Open(t))
	other := cron("c2", "dev", "billing")
	other.ServiceID = "other"
	require.NoError(t, repo.Create(ctx, cron("c1", "dev", "nightly")))
	require.NoError(t, repo.Create(ctx, other))

	got, err := repo.ForService(ctx, "svc")

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "c1", got[0].ID)
}

func Test_Delete_RemovesRunsAndCommands(t *testing.T) {
	repo := mysql.NewCronRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, cron("c1", "dev", "nightly")))
	require.NoError(t, repo.RecordRun(ctx, run("r1", "c1", at)))
	require.NoError(t, repo.AddCommand(ctx, entities.Command{ID: "k1", CronID: "c1", Kind: entities.CommandRun, CreatedAt: at}))

	require.NoError(t, repo.Delete(ctx, "c1"))

	_, err := repo.GetByID(ctx, "c1")
	require.ErrorIs(t, err, apperror.NotFound(""))
	runs, _ := repo.Runs(ctx, "c1", 50)
	require.Empty(t, runs)
	pending, _ := repo.PendingCommands(ctx)
	require.Empty(t, pending)
}
