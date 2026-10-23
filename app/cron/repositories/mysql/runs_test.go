package mysql_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/app/cron/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/stretchr/testify/require"
)

func run(id, cronID string, scheduled time.Time) entities.Run {
	return entities.Run{
		ID: id, CronID: cronID, Status: entities.RunSucceeded, ScheduledAt: scheduled,
		StartedAt: scheduled, FinishedAt: scheduled.Add(time.Second), Attempts: 1, LastStatus: 200,
	}
}

func Test_RecordRun_DropsADuplicateReport(t *testing.T) {
	repo := mysql.NewCronRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, cron("c1", "dev", "nightly")))
	first := run("r1", "c1", at)
	again := first
	again.Status = entities.RunFailed

	require.NoError(t, repo.RecordRun(ctx, first))
	require.NoError(t, repo.RecordRun(ctx, again))

	runs, err := repo.Runs(ctx, "c1", 50)
	require.NoError(t, err)
	require.Equal(t, []entities.Run{first}, runs)
}

func Test_RecordRun_IgnoresAnUnknownCron(t *testing.T) {
	repo := mysql.NewCronRepository(testdb.Open(t))

	require.NoError(t, repo.RecordRun(ctx, run("r1", "gone", at)))

	runs, _ := repo.Runs(ctx, "gone", 50)
	require.Empty(t, runs)
}

func Test_RecordRun_KeepsTheNewest50(t *testing.T) {
	repo := mysql.NewCronRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, cron("c1", "dev", "nightly")))
	require.NoError(t, repo.Create(ctx, cron("c2", "dev", "other")))
	require.NoError(t, repo.RecordRun(ctx, run("other", "c2", at)))

	for i := range 55 {
		require.NoError(t, repo.RecordRun(ctx, run(fmt.Sprintf("r%02d", i), "c1", at.Add(time.Duration(i)*time.Minute))))
	}

	runs, _ := repo.Runs(ctx, "c1", 100)
	require.Len(t, runs, entities.KeptRuns)
	require.Equal(t, "r54", runs[0].ID)
	require.Equal(t, "r05", runs[49].ID)
	others, _ := repo.Runs(ctx, "c2", 100)
	require.Len(t, others, 1)
}

func Test_RecordRun_CutsALongError(t *testing.T) {
	repo := mysql.NewCronRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, cron("c1", "dev", "nightly")))
	failed := run("r1", "c1", at)
	failed.Status, failed.Error = entities.RunFailed, strings.Repeat("é", 600)

	require.NoError(t, repo.RecordRun(ctx, failed))

	runs, _ := repo.Runs(ctx, "c1", 1)
	require.Equal(t, strings.Repeat("é", 512), runs[0].Error)
}

func Test_Commands_PendingUntilFinished(t *testing.T) {
	repo := mysql.NewCronRepository(testdb.Open(t))
	require.NoError(t, repo.AddCommand(ctx, entities.Command{ID: "k1", CronID: "c1", Kind: entities.CommandRun, CreatedAt: at}))
	require.NoError(t, repo.AddCommand(ctx, entities.Command{ID: "k2", CronID: "c1", Kind: entities.CommandRun, CreatedAt: at.Add(time.Second)}))

	require.NoError(t, repo.FinishCommand(ctx, "k1", at))

	pending, err := repo.PendingCommands(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "k2", pending[0].ID)
}
