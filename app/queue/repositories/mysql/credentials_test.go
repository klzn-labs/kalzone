package mysql_test

import (
	"testing"

	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/app/queue/repositories/mysql"
	"github.com/adrianozp/platform/internal/testdb"
	"github.com/stretchr/testify/require"
)

func Test_Credentials_SaveReplacesAndReads(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	c := entities.Credentials{
		ServiceID: "svc", Environment: "prod", TokenCiphertext: []byte("t"), TokenNonce: []byte("n"),
		TokenHash: entities.HashToken("token"), KeyCiphertext: []byte("k"), KeyNonce: []byte("n"), KeyID: "k1",
	}
	require.NoError(t, repo.SaveCredentials(ctx, c))
	c.PreviousKeyCiphertext, c.PreviousKeyNonce, c.RotatedAt = []byte("k"), []byte("n"), &at
	c.KeyCiphertext = []byte("k2")
	require.NoError(t, repo.SaveCredentials(ctx, c))

	got, found, err := repo.Credentials(ctx, "svc", "prod")
	_, missing, _ := repo.Credentials(ctx, "svc", "dev")
	all, _ := repo.AllCredentials(ctx)

	require.NoError(t, err)
	require.True(t, found)
	require.False(t, missing)
	require.Equal(t, []byte("k2"), got.KeyCiphertext)
	require.Equal(t, []byte("k"), got.PreviousKeyCiphertext)
	require.Len(t, all, 1)
}

func Test_CreateCredentials_KeepsTheFirstOfTwoConcurrentIssues(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	first := entities.Credentials{ServiceID: "svc", Environment: "prod", TokenCiphertext: []byte("1"), TokenNonce: []byte("n"), TokenHash: "h1", KeyCiphertext: []byte("1"), KeyNonce: []byte("n"), KeyID: "k1"}
	second := first
	second.TokenHash, second.KeyCiphertext = "h2", []byte("2")

	created, err := repo.CreateCredentials(ctx, first)
	again, againErr := repo.CreateCredentials(ctx, second)

	stored, _, _ := repo.Credentials(ctx, "svc", "prod")
	require.NoError(t, err)
	require.NoError(t, againErr)
	require.True(t, created)
	require.False(t, again)
	require.Equal(t, "h1", stored.TokenHash)
}

func Test_DeleteCredentials_RevokesEveryEnvironmentOfTheService(t *testing.T) {
	repo := mysql.NewQueueRepository(testdb.Open(t))
	for _, key := range [][2]string{{"svc", "dev"}, {"svc", "prod"}, {"other", "prod"}} {
		require.NoError(t, repo.SaveCredentials(ctx, entities.Credentials{
			ServiceID: key[0], Environment: key[1], TokenCiphertext: []byte("t"), TokenNonce: []byte("n"),
			TokenHash: entities.HashToken(key[0] + key[1]), KeyCiphertext: []byte("k"), KeyNonce: []byte("n"), KeyID: "k1",
		}))
	}

	require.NoError(t, repo.DeleteCredentials(ctx, "svc"))
	all, err := repo.AllCredentials(ctx)

	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, "other", all[0].ServiceID)
}
