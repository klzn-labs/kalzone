package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/app/release/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/klzn-labs/kalzone/pkg/apperror"
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
	list, err := repo.ListByService(ctx, "a", 0, 10)
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

func Test_ListByService_ContinuesBelowANumber(t *testing.T) {
	repo := mysql.NewReleaseRepository(testdb.Open(t))
	now := time.Now().UTC()
	for _, id := range []string{"a1", "a2", "a3"} {
		_, err := repo.Create(ctx, requested(id, "a", now))
		require.NoError(t, err)
	}

	first, err := repo.ListByService(ctx, "a", 0, 2)
	require.NoError(t, err)
	rest, err := repo.ListByService(ctx, "a", first[1].Number, 2)
	require.NoError(t, err)

	require.Equal(t, []string{"a3", "a2"}, []string{first[0].ID, first[1].ID})
	require.Len(t, rest, 1)
	require.Equal(t, "a1", rest[0].ID)
}

func Test_UnfinishedFor_ListsOnlyTheServicesOpenBuilds(t *testing.T) {
	repo := mysql.NewReleaseRepository(testdb.Open(t))
	now := time.Now().UTC()
	open, _ := repo.Create(ctx, requested("a1", "a", now))
	done, _ := repo.Create(ctx, requested("a2", "a", now))
	done.Status = entities.StatusFailed
	require.NoError(t, repo.Transition(ctx, done, entities.StatusRequested))
	_, _ = repo.Create(ctx, requested("b1", "b", now))

	unfinished, err := repo.UnfinishedFor(ctx, "a")

	require.NoError(t, err)
	require.Len(t, unfinished, 1)
	require.Equal(t, open.ID, unfinished[0].ID)
}

func builtAt(t *testing.T, repo mysql.ReleaseRepository, id, commit string, at time.Time) entities.Release {
	t.Helper()
	rel, err := repo.Create(ctx, requested(id, "a", at))
	require.NoError(t, err)
	rel.Status, rel.Digest, rel.CommitSHA, rel.FinishedAt = entities.StatusBuilt, "sha256:"+id, commit, &at
	require.NoError(t, repo.Transition(ctx, rel, entities.StatusRequested))
	return rel
}

func Test_PreviousBuilt_ByFinishTimeNotNumber(t *testing.T) {
	repo := mysql.NewReleaseRepository(testdb.Open(t))
	t0 := time.Now().UTC().Truncate(time.Millisecond)
	builtAt(t, repo, "r1", sha("1"), t0)
	r2 := builtAt(t, repo, "r2", sha("2"), t0.Add(3*time.Minute))
	builtAt(t, repo, "r3", sha("3"), t0.Add(2*time.Minute))
	builtAt(t, repo, "r4", "", t0.Add(150*time.Second))

	previous, found, err := repo.PreviousBuilt(ctx, "a", r2.ID, *r2.FinishedAt)

	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "r3", previous.ID)
}

func Test_PreviousBuilt_NoneForTheFirst(t *testing.T) {
	repo := mysql.NewReleaseRepository(testdb.Open(t))
	r1 := builtAt(t, repo, "r1", sha("1"), time.Now().UTC())

	_, found, err := repo.PreviousBuilt(ctx, "a", r1.ID, *r1.FinishedAt)

	require.NoError(t, err)
	require.False(t, found)
}
