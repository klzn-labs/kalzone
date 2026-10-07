package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/release/entities"
	"github.com/adrianozp/platform/app/release/repositories/mysql"
	"github.com/adrianozp/platform/internal/testdb"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func requested(id, service string, at time.Time) entities.Release {
	return entities.Release{
		ID: id, ServiceID: service, Source: entities.SourceBuild, GitRef: "main", Image: "docker.io/a/b",
		Status: entities.StatusRequested, Trigger: entities.TriggerManual, CreatedAt: at,
	}
}

func Test_Create_NumbersReleasesPerService(t *testing.T) {
	repo := mysql.NewReleaseRepository(testdb.Open(t))
	now := time.Now().UTC()

	a1, err := repo.Create(ctx, requested("a1", "a", now))
	require.NoError(t, err)
	a2, _ := repo.Create(ctx, requested("a2", "a", now))
	b1, _ := repo.Create(ctx, requested("b1", "b", now))

	require.Equal(t, []int{1, 2, 1}, []int{a1.Number, a2.Number, b1.Number})
	list, err := repo.ListByService(ctx, "a", 10)
	require.NoError(t, err)
	require.Equal(t, "a2", list[0].ID)
}

func Test_Transition_OnlyFromTheExpectedStatus(t *testing.T) {
	repo := mysql.NewReleaseRepository(testdb.Open(t))
	rel, err := repo.Create(ctx, requested("r1", "a", time.Now().UTC()))
	require.NoError(t, err)

	rel.Status, rel.Digest = entities.StatusBuilt, "sha256:abc"
	require.NoError(t, repo.Transition(ctx, rel, entities.StatusRequested))
	again := repo.Transition(ctx, rel, entities.StatusRequested)

	stored, _ := repo.FindByID(ctx, "r1")
	require.Equal(t, entities.StatusBuilt, stored.Status)
	require.Equal(t, "sha256:abc", stored.Digest)
	require.ErrorIs(t, again, apperror.Conflict(""))
}

func Test_Unfinished_FindsOldPendingReleases(t *testing.T) {
	repo := mysql.NewReleaseRepository(testdb.Open(t))
	old := time.Now().UTC().Add(-2 * time.Hour)
	_, err := repo.Create(ctx, requested("old", "a", old))
	require.NoError(t, err)
	_, _ = repo.Create(ctx, requested("new", "a", time.Now().UTC()))

	stale, err := repo.Unfinished(ctx, time.Now().UTC().Add(-time.Hour))

	require.NoError(t, err)
	require.Len(t, stale, 1)
	require.Equal(t, "old", stale[0].ID)
}
