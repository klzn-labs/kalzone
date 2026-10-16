package deployments_test

import (
	"testing"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/app/deploy/usecases/deployments"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_AutoDeploy_SchedulesTheReleaseOnDevWithItsCurrentConfig(t *testing.T) {
	_, usecase := setup("dev")

	d, err := usecase.AutoDeploy(ctx, "s1", "r2")

	require.NoError(t, err)
	require.Equal(t, "dev", d.Environment)
	require.Equal(t, entities.TriggerPush, d.Trigger)
	require.Equal(t, entities.StatusPending, d.Status)
	require.Equal(t, 4, d.ConfigRevision)
	require.Equal(t, "img@sha256:2", d.ImageRef)
	require.Equal(t, deployments.AutoDeployActor.UserID, d.RequestedBy)
}

func Test_AutoDeploy_RefusesReleasesItCannotRun(t *testing.T) {
	_, usecase := setup("")

	_, unbuilt := usecase.AutoDeploy(ctx, "s1", "wip")
	_, foreign := usecase.AutoDeploy(ctx, "s1", "other")
	_, missing := usecase.AutoDeploy(ctx, "s404", "r1")

	require.ErrorIs(t, unbuilt, apperror.Invalid(""))
	require.ErrorIs(t, foreign, apperror.Invalid(""))
	require.ErrorIs(t, missing, apperror.NotFound(""))
}

func Test_AutoDeploy_LeavesARunningDeployAlone(t *testing.T) {
	store, usecase := setup("")
	running, _ := usecase.AutoDeploy(ctx, "s1", "r1")
	running.Status = entities.StatusInProgress
	store.ByID[running.ID] = running

	_, err := usecase.AutoDeploy(ctx, "s1", "r2")

	require.ErrorIs(t, err, apperror.Conflict(""))
}
