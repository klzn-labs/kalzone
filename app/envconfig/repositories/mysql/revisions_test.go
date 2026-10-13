package mysql_test

import (
	"context"
	"fmt"
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

func Test_HostsTaken_OnlyCountsOtherPairsCurrentRevisions(t *testing.T) {
	repo := mysql.NewConfigRepository(testdb.Open(t))
	require.NoError(t, repo.Insert(ctx, revision("r1", "a", "dev", 1, "old.kalz.one")))
	require.NoError(t, repo.Insert(ctx, revision("r2", "a", "dev", 2, "new.kalz.one")))
	require.NoError(t, repo.Insert(ctx, revision("r3", "b", "prod", 1, "b.kalz.one")))

	cases := map[string]bool{"old.kalz.one": false, "new.kalz.one": true, "b.kalz.one": true}
	for host, want := range cases {
		taken, err := repo.HostsTaken(ctx, []string{host}, "c", "dev")
		require.NoError(t, err)
		require.Equal(t, want, taken, host)
	}
	taken, err := repo.HostsTaken(ctx, []string{"new.kalz.one"}, "a", "dev")
	require.NoError(t, err)
	require.False(t, taken)
}

func Test_HostsTaken_CountsOtherPairsAliases(t *testing.T) {
	repo := mysql.NewConfigRepository(testdb.Open(t))
	web := revision("r1", "web", "prod", 1, "kalz.one")
	web.Spec.Aliases = []string{"www.kalz.one"}
	require.NoError(t, repo.Insert(ctx, web))

	takenByOther, err := repo.HostsTaken(ctx, []string{"new.kalz.one", "www.kalz.one"}, "c", "prod")
	require.NoError(t, err)
	ownAlias, err := repo.HostsTaken(ctx, []string{"www.kalz.one"}, "web", "prod")
	require.NoError(t, err)

	require.True(t, takenByOther)
	require.False(t, ownAlias)
}

func Test_InternalTaken_OnlyCountsOtherServicesInTheEnvironment(t *testing.T) {
	repo := mysql.NewConfigRepository(testdb.Open(t))
	api := revision("r1", "api", "prod", 1, "")
	api.Spec.Internal = true
	require.NoError(t, repo.Insert(ctx, api))

	cases := map[[2]string]bool{{"other", "prod"}: true, {"api", "prod"}: false, {"other", "dev"}: false}
	for pair, want := range cases {
		taken, err := repo.InternalTaken(ctx, pair[0], pair[1])
		require.NoError(t, err)
		require.Equal(t, want, taken, pair)
	}
}

func Test_ByNumber(t *testing.T) {
	repo := mysql.NewConfigRepository(testdb.Open(t))
	require.NoError(t, repo.Insert(ctx, revision("r1", "svc", "dev", 1, "a.kalz.one")))
	require.NoError(t, repo.Insert(ctx, revision("r2", "svc", "dev", 2, "b.kalz.one")))

	first, err := repo.ByNumber(ctx, "svc", "dev", 1)
	_, missing := repo.ByNumber(ctx, "svc", "dev", 9)

	require.NoError(t, err)
	require.Equal(t, "a.kalz.one", first.Spec.Host)
	require.ErrorIs(t, missing, apperror.NotFound(""))
}

func Test_History_ContinuesBelowANumber(t *testing.T) {
	repo := mysql.NewConfigRepository(testdb.Open(t))
	for number := 1; number <= 3; number++ {
		require.NoError(t, repo.Insert(ctx, revision(fmt.Sprintf("r%d", number), "svc", "dev", number, "")))
	}

	first, err := repo.History(ctx, "svc", "dev", 0, 2)
	require.NoError(t, err)
	rest, err := repo.History(ctx, "svc", "dev", first[1].Number, 2)
	require.NoError(t, err)

	require.Equal(t, []int{3, 2}, []int{first[0].Number, first[1].Number})
	require.Len(t, rest, 1)
	require.Equal(t, 1, rest[0].Number)
}
