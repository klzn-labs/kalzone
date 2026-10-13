package mysql_test

import (
	"testing"

	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/app/envconfig/repositories/mysql"
	"github.com/adrianozp/platform/internal/testdb"
	"github.com/stretchr/testify/require"
)

func Test_Search_FindsCurrentEnvValuesAndSecretNamesOnly(t *testing.T) {
	repo := mysql.NewConfigRepository(testdb.Open(t))
	old := revision("r1", "api", "dev", 1, "")
	old.Env["LOG_LEVEL"] = "debug"
	current := revision("r2", "api", "dev", 2, "")
	current.Env["LOG_LEVEL"] = "info"
	secret := revision("r3", "worker", "prod", 1, "")
	secret.Secrets["LOG_LEVEL"] = "version-id"
	for _, rev := range []entities.Revision{old, current, secret, revision("r4", "web", "dev", 1, "")} {
		require.NoError(t, repo.Insert(ctx, rev))
	}

	all, err := repo.Search(ctx, "LOG_LEVEL", "")
	require.NoError(t, err)
	prod, err := repo.Search(ctx, "LOG_LEVEL", "prod")
	require.NoError(t, err)

	require.Equal(t, []entities.Match{
		{ServiceID: "api", Environment: "dev", Revision: 2, Kind: entities.MatchEnv, Value: "info"},
		{ServiceID: "worker", Environment: "prod", Revision: 1, Kind: entities.MatchSecret},
	}, all)
	require.Len(t, prod, 1)
	require.Equal(t, "worker", prod[0].ServiceID)
}

func Test_Search_KeysWithQuotesOrBackslashesMatchNothingInsteadOfFailing(t *testing.T) {
	repo := mysql.NewConfigRepository(testdb.Open(t))
	require.NoError(t, repo.Insert(ctx, revision("r1", "api", "dev", 1, "")))

	quoted, quotedErr := repo.Search(ctx, `A"B`, "")
	slashed, slashedErr := repo.Search(ctx, `A\B`, "")

	require.NoError(t, quotedErr)
	require.NoError(t, slashedErr)
	require.Empty(t, quoted)
	require.Empty(t, slashed)
}
