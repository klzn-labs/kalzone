package mysql_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/stretchr/testify/require"
)

func status(env, state string, at time.Time) entities.RuntimeStatus {
	started := at.Add(-time.Hour)
	return entities.RuntimeStatus{
		ServiceID: "s", Environment: env, Container: "api-" + env + "-blue", Found: true, State: state, Health: "healthy",
		HealthLog: []string{"ok"}, Restarts: 1, Image: "img@sha256:x", MemoryLimitMB: 128, CPULimitMillis: 250,
		StartedAt: &started, ObservedAt: at, Agent: "vps-1",
	}
}

func Test_SaveRuntimeKeepsOneRowPerServiceAndEnvironment(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))
	at := time.Now().UTC().Truncate(time.Millisecond)

	require.NoError(t, repo.SaveRuntime(ctx, []entities.RuntimeStatus{status("dev", "running", at), status("prod", "running", at)}))
	require.NoError(t, repo.SaveRuntime(ctx, []entities.RuntimeStatus{status("dev", "exited", at.Add(time.Second))}))
	dev, found, err := repo.Runtime(ctx, "s", "dev")

	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "exited", dev.State)
	require.Equal(t, []string{"ok"}, dev.HealthLog)
	require.Equal(t, at.Add(time.Second), dev.ObservedAt)
	require.Equal(t, 128, dev.MemoryLimitMB)
}

func Test_RuntimeOfAnUnreportedServiceIsNotFound(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))

	_, found, err := repo.Runtime(ctx, "s", "dev")

	require.NoError(t, err)
	require.False(t, found)
}

func Test_AFailedTransitionKeepsItsFailure(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))
	d, err := repo.Create(ctx, deployment("d1", "s", "dev", entities.StatusInProgress, time.Now().UTC()))
	require.NoError(t, err)
	failure := &entities.Failure{Class: entities.FailureUnhealthy, Container: "api-dev-green", ExitCode: 0, HealthLog: []string{"404"}, Logs: []string{"listening"}}

	d.Status, d.Failure = entities.StatusFailed, failure
	require.NoError(t, repo.Transition(ctx, d, entities.StatusInProgress))
	stored, _ := repo.FindByID(ctx, "d1")
	fresh, _ := repo.Create(ctx, deployment("d2", "s", "dev", entities.StatusPending, time.Now().UTC()))

	require.Equal(t, failure, stored.Failure)
	require.Nil(t, fresh.Failure)
}

func Test_DeleteRuntimeForgetsEveryEnvironmentOfTheService(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))
	at := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.SaveRuntime(ctx, []entities.RuntimeStatus{status("dev", "running", at), status("prod", "running", at)}))

	require.NoError(t, repo.DeleteRuntime(ctx, "s"))
	_, devFound, _ := repo.Runtime(ctx, "s", "dev")
	_, prodFound, _ := repo.Runtime(ctx, "s", "prod")

	require.False(t, devFound)
	require.False(t, prodFound)
}
