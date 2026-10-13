package mysql_test

import (
	"testing"
	"time"

	"github.com/adrianozp/platform/app/release/entities"
	"github.com/adrianozp/platform/app/release/repositories/mysql"
	"github.com/adrianozp/platform/internal/testdb"
	"github.com/stretchr/testify/require"
)

const (
	shaA = "abc1234000000000000000000000000000000000"
	shaB = "abc1234999999999999999999999999999999999"
	shaC = "def5678000000000000000000000000000000000"
)

func builtWith(t *testing.T, repo mysql.ReleaseRepository, id, sha, tag string) entities.Release {
	t.Helper()
	rel, err := repo.Create(ctx, requested(id, "a", time.Now().UTC()))
	require.NoError(t, err)
	rel.Status, rel.CommitSHA, rel.Tag, rel.Digest = entities.StatusBuilt, sha, tag, "sha256:x"
	require.NoError(t, repo.Transition(ctx, rel, entities.StatusRequested))
	return rel
}

func Test_LatestBuilt_SkipsReleasesThatAreNotBuilt(t *testing.T) {
	repo := mysql.NewReleaseRepository(testdb.Open(t))
	builtWith(t, repo, "a1", shaA, "1.0.0")
	_, err := repo.Create(ctx, requested("a2", "a", time.Now().UTC()))
	require.NoError(t, err)

	latest, found, err := repo.LatestBuilt(ctx, "a")
	_, none, noneErr := repo.LatestBuilt(ctx, "b")

	require.NoError(t, err)
	require.NoError(t, noneErr)
	require.True(t, found)
	require.Equal(t, "a1", latest.ID)
	require.False(t, none)
}

func Test_ByNumber_FindsAnyStatusWithinTheService(t *testing.T) {
	repo := mysql.NewReleaseRepository(testdb.Open(t))
	_, err := repo.Create(ctx, requested("a1", "a", time.Now().UTC()))
	require.NoError(t, err)

	rel, found, err := repo.ByNumber(ctx, "a", 1)
	_, other, otherErr := repo.ByNumber(ctx, "b", 1)

	require.NoError(t, err)
	require.NoError(t, otherErr)
	require.True(t, found)
	require.Equal(t, "a1", rel.ID)
	require.False(t, other)
}

func Test_BuiltByCommitPrefix_ListsNewestFirst(t *testing.T) {
	repo := mysql.NewReleaseRepository(testdb.Open(t))
	builtWith(t, repo, "a1", shaA, "")
	builtWith(t, repo, "a2", shaB, "")
	builtWith(t, repo, "a3", shaC, "")

	matches, err := repo.BuiltByCommitPrefix(ctx, "a", "abc1234", 10)

	require.NoError(t, err)
	require.Equal(t, []string{"a2", "a1"}, []string{matches[0].ID, matches[1].ID})
}

func Test_BuiltByTag_FindsTheNewestBuiltWithTheTag(t *testing.T) {
	repo := mysql.NewReleaseRepository(testdb.Open(t))
	builtWith(t, repo, "a1", shaA, "1.0.0")
	builtWith(t, repo, "a2", shaB, "1.0.0")

	rel, found, err := repo.BuiltByTag(ctx, "a", "1.0.0")
	_, missing, missingErr := repo.BuiltByTag(ctx, "a", "2.0.0")

	require.NoError(t, err)
	require.NoError(t, missingErr)
	require.True(t, found)
	require.Equal(t, "a2", rel.ID)
	require.False(t, missing)
}

func Test_BuiltByCommitPrefix_TreatsWildcardsAsLiterals(t *testing.T) {
	repo := mysql.NewReleaseRepository(testdb.Open(t))
	builtWith(t, repo, "a1", shaA, "")

	percent, percentErr := repo.BuiltByCommitPrefix(ctx, "a", "%", 10)
	underscore, underscoreErr := repo.BuiltByCommitPrefix(ctx, "a", "_bc", 10)
	backslash, backslashErr := repo.BuiltByCommitPrefix(ctx, "a", `\`, 10)

	require.NoError(t, percentErr)
	require.NoError(t, underscoreErr)
	require.NoError(t, backslashErr)
	require.Empty(t, percent)
	require.Empty(t, underscore)
	require.Empty(t, backslash)
}
