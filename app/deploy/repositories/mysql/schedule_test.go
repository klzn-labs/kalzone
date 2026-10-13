package mysql_test

import (
	"testing"
	"time"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/app/deploy/repositories/mysql"
	"github.com/adrianozp/platform/internal/testdb"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_ScheduleAll_SupersedesPendingAndNumbersEachTarget(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))
	now := time.Now().UTC()
	_, err := repo.Create(ctx, deployment("old", "api", "dev", entities.StatusPending, now))
	require.NoError(t, err)

	created, err := repo.ScheduleAll(ctx, []entities.Deployment{
		deployment("n1", "api", "dev", entities.StatusPending, now),
		deployment("n2", "worker", "dev", entities.StatusPending, now),
	})

	require.NoError(t, err)
	require.Equal(t, []int{2, 1}, []int{created[0].Number, created[1].Number})
	old, _ := repo.FindByID(ctx, "old")
	require.Equal(t, entities.StatusCancelled, old.Status)
	require.Equal(t, "superseded before deployment #1 started", old.Message)
}

func Test_ScheduleAll_WritesNothingWhenOneTargetIsRunning(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))
	now := time.Now().UTC()
	_, err := repo.Create(ctx, deployment("busy", "worker", "dev", entities.StatusInProgress, now))
	require.NoError(t, err)

	_, err = repo.ScheduleAll(ctx, []entities.Deployment{
		deployment("n1", "api", "dev", entities.StatusPending, now),
		deployment("n2", "worker", "dev", entities.StatusPending, now),
	})

	require.ErrorIs(t, err, apperror.Conflict(""))
	_, missing := repo.FindByID(ctx, "n1")
	require.ErrorIs(t, missing, apperror.NotFound(""))
}

func Test_ScheduleAll_RejectsTheSameTargetTwiceAndWritesNothing(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))
	now := time.Now().UTC()

	_, err := repo.ScheduleAll(ctx, []entities.Deployment{
		deployment("n1", "api", "dev", entities.StatusPending, now),
		deployment("n2", "api", "dev", entities.StatusPending, now),
	})

	require.ErrorIs(t, err, apperror.Invalid(""))
	_, missing := repo.FindByID(ctx, "n1")
	require.ErrorIs(t, missing, apperror.NotFound(""))
}
