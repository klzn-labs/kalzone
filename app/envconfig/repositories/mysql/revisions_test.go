package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/app/envconfig/repositories/mysql"
	"github.com/adrianozp/platform/internal/testdb"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func revision(id, service, env string, number int, host string) entities.Revision {
	rev := entities.Initial(service, env)
	rev.ID, rev.Number, rev.AuthorID = id, number, "author"
	rev.Spec.Host = host
	rev.Env["A"] = "1"
	rev.CreatedAt = time.Date(2026, 10, 7, 0, 0, number, 0, time.UTC)
	return rev
}

func Test_Latest_ReturnsHighestNumberWithMaps(t *testing.T) {
	repo := mysql.NewConfigRepository(testdb.Open(t))
	require.NoError(t, repo.Insert(ctx, revision("r1", "svc", "dev", 1, "")))
	require.NoError(t, repo.Insert(ctx, revision("r2", "svc", "dev", 2, "a.kalz.one")))

	latest, found, err := repo.Latest(ctx, "svc", "dev")

	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "r2", latest.ID)
	require.Equal(t, "a.kalz.one", latest.Spec.Host)
	require.Equal(t, map[string]string{"A": "1"}, latest.Env)
}

func Test_Latest_ReportsMissing(t *testing.T) {
	repo := mysql.NewConfigRepository(testdb.Open(t))

	_, found, err := repo.Latest(ctx, "svc", "dev")

	require.NoError(t, err)
	require.False(t, found)
}

func Test_Insert_SameNumberTwiceIsAConflict(t *testing.T) {
	repo := mysql.NewConfigRepository(testdb.Open(t))
	require.NoError(t, repo.Insert(ctx, revision("r1", "svc", "dev", 1, "")))

	err := repo.Insert(ctx, revision("r2", "svc", "dev", 1, ""))

	require.ErrorIs(t, err, apperror.Conflict(""))
}

func Test_HostTaken_OnlyCountsOtherPairsCurrentRevisions(t *testing.T) {
	repo := mysql.NewConfigRepository(testdb.Open(t))
	require.NoError(t, repo.Insert(ctx, revision("r1", "a", "dev", 1, "old.kalz.one")))
	require.NoError(t, repo.Insert(ctx, revision("r2", "a", "dev", 2, "new.kalz.one")))
	require.NoError(t, repo.Insert(ctx, revision("r3", "b", "prod", 1, "b.kalz.one")))

	cases := map[string]bool{"old.kalz.one": false, "new.kalz.one": true, "b.kalz.one": true}
	for host, want := range cases {
		taken, err := repo.HostTaken(ctx, host, "c", "dev")
		require.NoError(t, err)
		require.Equal(t, want, taken, host)
	}
	taken, err := repo.HostTaken(ctx, "new.kalz.one", "a", "dev")
	require.NoError(t, err)
	require.False(t, taken)
}
