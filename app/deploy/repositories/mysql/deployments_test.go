package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/app/deploy/repositories/mysql"
	"github.com/adrianozp/platform/internal/testdb"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func deployment(id, service, env string, status entities.Status, at time.Time) entities.Deployment {
	return entities.Deployment{
		ID: id, ServiceID: service, Environment: env, ReleaseID: "r", ReleaseNumber: 1, ImageRef: "img@sha256:x",
		Strategy: entities.StrategyBlueGreen, Trigger: entities.TriggerManual, Status: status, CreatedAt: at,
	}
}

func Test_LatestSucceededIsTheDesiredState(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))
	now := time.Now().UTC()
	for _, d := range []entities.Deployment{
		deployment("d1", "s", "dev", entities.StatusSucceeded, now),
		deployment("d2", "s", "dev", entities.StatusSucceeded, now),
		deployment("d3", "s", "dev", entities.StatusFailed, now),
		deployment("p1", "s", "prod", entities.StatusSucceeded, now),
	} {
		_, err := repo.Create(ctx, d)
		require.NoError(t, err)
	}

	live, found, err := repo.LatestSucceeded(ctx, "s", "dev")
	all, allErr := repo.AllLive(ctx)

	require.NoError(t, err)
	require.NoError(t, allErr)
	require.True(t, found)
	require.Equal(t, "d2", live.ID)
	require.Equal(t, 2, live.Number)
	require.Len(t, all, 2)
}

func Test_TransitionIsACompareAndSet(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))
	d, err := repo.Create(ctx, deployment("d1", "s", "dev", entities.StatusPending, time.Now().UTC()))
	require.NoError(t, err)

	d.Status, d.Agent = entities.StatusInProgress, "vps-1"
	first := repo.Transition(ctx, d, entities.StatusPending)
	second := repo.Transition(ctx, d, entities.StatusPending)

	require.NoError(t, first)
	require.ErrorIs(t, second, apperror.Conflict(""))
	stored, _ := repo.FindByID(ctx, "d1")
	require.Equal(t, "vps-1", stored.Agent)
}

func Test_EventsComeBackInOrder(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))
	now := time.Now().UTC()
	require.NoError(t, repo.AppendEvents(ctx, "d1", now, []string{"one", "two"}))
	require.NoError(t, repo.AppendEvents(ctx, "d1", now, []string{"three"}))

	events, err := repo.Events(ctx, "d1", 2)

	require.NoError(t, err)
	require.Equal(t, "two", events[0].Message)
	require.Equal(t, "three", events[1].Message)
}

func Test_TouchAgentUpserts(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))
	require.NoError(t, repo.TouchAgent(ctx, entities.Agent{Name: "vps-1", Version: "a", LastSeenAt: time.Now().UTC()}))
	require.NoError(t, repo.TouchAgent(ctx, entities.Agent{Name: "vps-1", Version: "b", LastSeenAt: time.Now().UTC()}))

	agents, err := repo.Agents(ctx)

	require.NoError(t, err)
	require.Len(t, agents, 1)
	require.Equal(t, "b", agents[0].Version)
}
