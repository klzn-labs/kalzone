package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/klzn-labs/kalzone/pkg/apperror"
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

func Test_HistoryContinuesBelowANumber(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))
	now := time.Now().UTC()
	for _, id := range []string{"d1", "d2", "d3"} {
		_, err := repo.Create(ctx, deployment(id, "s1", "dev", entities.StatusSucceeded, now))
		require.NoError(t, err)
	}

	first, err := repo.History(ctx, "s1", "dev", 0, 2)
	require.NoError(t, err)
	rest, err := repo.History(ctx, "s1", "dev", first[1].Number, 2)
	require.NoError(t, err)

	require.Equal(t, []int{3, 2}, []int{first[0].Number, first[1].Number})
	require.Len(t, rest, 1)
	require.Equal(t, 1, rest[0].Number)
}

func Test_EventsAfterReadsForwardAndCounts(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))
	now := time.Now().UTC()
	require.NoError(t, repo.AppendEvents(ctx, "d1", now, []string{"one", "two", "three"}))
	require.NoError(t, repo.AppendEvents(ctx, "d2", now, []string{"other"}))

	first, err := repo.EventsAfter(ctx, "d1", 0, 2)
	require.NoError(t, err)
	rest, err := repo.EventsAfter(ctx, "d1", first[1].ID, 2)
	require.NoError(t, err)
	total, err := repo.CountEvents(ctx, "d1")

	require.NoError(t, err)
	require.Equal(t, []string{"one", "two"}, []string{first[0].Message, first[1].Message})
	require.Len(t, rest, 1)
	require.Equal(t, "three", rest[0].Message)
	require.Equal(t, 3, total)
}

func Test_EnvironmentsListsEveryEnvironmentAServiceWasDeployedTo(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))
	now := time.Now().UTC()
	for _, d := range []entities.Deployment{
		deployment("d1", "s", "prod", entities.StatusFailed, now),
		deployment("d2", "s", "dev", entities.StatusSucceeded, now),
		deployment("d3", "s", "dev", entities.StatusSucceeded, now),
		deployment("o1", "other", "qa", entities.StatusSucceeded, now),
	} {
		_, err := repo.Create(ctx, d)
		require.NoError(t, err)
	}

	environments, err := repo.Environments(ctx, "s")

	require.NoError(t, err)
	require.Equal(t, []string{"dev", "prod"}, environments)
}

func Test_FinishedSinceKeepsFinishedDeploysOfOneEnvironmentInOrder(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))
	now := time.Now().UTC().Truncate(time.Second)
	for _, d := range []entities.Deployment{
		deployment("late", "s", "prod", entities.StatusSucceeded, now),
		deployment("early", "t", "prod", entities.StatusFailed, now),
		deployment("old", "u", "prod", entities.StatusSucceeded, now),
		deployment("running", "v", "prod", entities.StatusInProgress, now),
		deployment("dev", "s", "dev", entities.StatusSucceeded, now),
	} {
		_, err := repo.Create(ctx, d)
		require.NoError(t, err)
	}
	finish(t, repo, "late", entities.StatusSucceeded, now.Add(-time.Hour))
	finish(t, repo, "early", entities.StatusFailed, now.Add(-2*time.Hour))
	finish(t, repo, "old", entities.StatusSucceeded, now.Add(-30*24*time.Hour))
	finish(t, repo, "dev", entities.StatusSucceeded, now.Add(-time.Hour))

	found, err := repo.FinishedSince(ctx, "prod", now.Add(-14*24*time.Hour))

	require.NoError(t, err)
	require.Equal(t, []string{"early", "late"}, []string{found[0].ID, found[1].ID})
	require.Len(t, found, 2)
}

func finish(t *testing.T, repo mysql.DeploymentRepository, id string, status entities.Status, at time.Time) {
	t.Helper()
	d, err := repo.FindByID(ctx, id)
	require.NoError(t, err)
	d.Status, d.FinishedAt = status, &at
	require.NoError(t, repo.Transition(ctx, d, d.Status))
}
