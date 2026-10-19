package mysql_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/app/release/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/stretchr/testify/require"
)

func changelog(id string, state entities.ChangelogState) entities.Changelog {
	return entities.Changelog{
		ReleaseID: id, BaseSHA: sha("a"), HeadSHA: sha("b"), State: state,
		Commits: []entities.Commit{}, UpdatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
}

func sha(c string) string {
	out := ""
	for len(out) < 40 {
		out += c
	}
	return out
}

func Test_Save_RoundTripsCommits(t *testing.T) {
	repo := mysql.NewChangelogRepository(testdb.Open(t))
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	c := changelog("r2", entities.ChangelogAhead)
	c.AheadBy, c.Truncated, c.CompareURL = 2, true, "https://github.com/o/api/compare/a...b"
	c.Commits = []entities.Commit{
		{SHA: sha("b"), Title: "feat: x", Type: "feat", Author: "ana", CommittedAt: at, PullRequest: &entities.PullRef{Number: 61, Title: "X", URL: "https://github.com/o/api/pull/61"}},
		{SHA: sha("c"), Title: "tidy", Type: "other", Author: "bo", CommittedAt: at},
	}

	require.NoError(t, repo.Save(ctx, c))
	stored, found, err := repo.Find(ctx, "r2")

	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, c, stored)
}

func Test_Find_MissingIsNotFound(t *testing.T) {
	repo := mysql.NewChangelogRepository(testdb.Open(t))

	_, found, err := repo.Find(ctx, "nope")

	require.NoError(t, err)
	require.False(t, found)
}

func Test_Save_Upserts(t *testing.T) {
	repo := mysql.NewChangelogRepository(testdb.Open(t))
	c := changelog("r2", entities.ChangelogPending)
	require.NoError(t, repo.Save(ctx, c))

	c.State, c.Attempts, c.LastError = entities.ChangelogFailed, 5, "boom"
	require.NoError(t, repo.Save(ctx, c))

	stored, _, err := repo.Find(ctx, "r2")
	require.NoError(t, err)
	require.Equal(t, c, stored)
}

func Test_FindMany_KeysByRelease(t *testing.T) {
	repo := mysql.NewChangelogRepository(testdb.Open(t))
	require.NoError(t, repo.Save(ctx, changelog("r1", entities.ChangelogFirst)))
	require.NoError(t, repo.Save(ctx, changelog("r2", entities.ChangelogAhead)))

	many, err := repo.FindMany(ctx, []string{"r1", "r2", "r3"})

	require.NoError(t, err)
	require.Len(t, many, 2)
	require.Equal(t, entities.ChangelogFirst, many["r1"].State)
}

func Test_Pending_OnlyPendingRows(t *testing.T) {
	repo := mysql.NewChangelogRepository(testdb.Open(t))
	require.NoError(t, repo.Save(ctx, changelog("r1", entities.ChangelogPending)))
	require.NoError(t, repo.Save(ctx, changelog("r2", entities.ChangelogAhead)))

	pending, err := repo.Pending(ctx)

	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "r1", pending[0].ReleaseID)
}

func Test_ByPair_IgnoresPendingAndFailed(t *testing.T) {
	repo := mysql.NewChangelogRepository(testdb.Open(t))
	require.NoError(t, repo.Save(ctx, changelog("r1", entities.ChangelogPending)))
	require.NoError(t, repo.Save(ctx, changelog("r2", entities.ChangelogFailed)))
	_, found, err := repo.ByPair(ctx, sha("a"), sha("b"))
	require.NoError(t, err)
	require.False(t, found)

	require.NoError(t, repo.Save(ctx, changelog("r3", entities.ChangelogAhead)))
	pair, found, err := repo.ByPair(ctx, sha("a"), sha("b"))

	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "r3", pair.ReleaseID)
}
