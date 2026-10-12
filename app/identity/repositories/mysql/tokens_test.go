package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/identity/entities"
	"github.com/adrianozp/platform/app/identity/repositories/mysql"
	"github.com/adrianozp/platform/internal/testdb"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/scope"
	"github.com/stretchr/testify/require"
)

var created = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

func token(id, owner, name string) entities.AccessToken {
	return entities.AccessToken{
		ID: id, OwnerID: owner, Name: name, Hash: "hash-" + id, Prefix: "abcd1234",
		Scopes: []scope.Scope{scope.Read, scope.Deploy}, TeamIDs: []string{"t1", "t2"},
		CreatedAt: created, ExpiresAt: created.Add(90 * 24 * time.Hour),
	}
}

func Test_Tokens_RoundTripAndLookups(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewTokenRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, token("k1", "u1", "claude-local")))
	require.NoError(t, repo.Create(ctx, token("k2", "u2", "claude-local")))

	byHash, err := repo.FindByHash(ctx, "hash-k1")
	require.NoError(t, err)
	mine, _ := repo.ListByOwner(ctx, "u1")
	all, _ := repo.ListAll(ctx)
	_, missing := repo.FindByHash(ctx, "nope")
	_, unknown := repo.FindByID(ctx, "nope")

	require.Equal(t, token("k1", "u1", "claude-local"), byHash)
	require.Len(t, mine, 1)
	require.Len(t, all, 2)
	require.ErrorIs(t, missing, entities.ErrTokenInvalid)
	require.ErrorIs(t, unknown, apperror.NotFound(""))
}

func Test_Tokens_NamesAreUniquePerOwnerWhileNotRevoked(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewTokenRepository(testdb.Open(t))
	expired := token("k1", "u1", "ci")
	expired.ExpiresAt = created.Add(-time.Hour)
	require.NoError(t, repo.Create(ctx, expired))

	taken := repo.Create(ctx, token("k2", "u1", "ci"))
	otherOwner := repo.Create(ctx, token("k3", "u2", "ci"))

	require.ErrorIs(t, taken, apperror.Conflict(""))
	require.NoError(t, otherOwner)
}

func Test_Tokens_ARevokedTokenFreesItsName(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewTokenRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, token("k1", "u1", "ci")))
	require.NoError(t, repo.Revoke(ctx, "k1", created))
	require.NoError(t, repo.Create(ctx, token("k2", "u1", "ci")))
	require.NoError(t, repo.Revoke(ctx, "k2", created))

	err := repo.Create(ctx, token("k3", "u1", "ci"))
	mine, _ := repo.ListByOwner(ctx, "u1")

	require.NoError(t, err)
	require.Len(t, mine, 3)
}

func Test_Tokens_RevokeAndTouch(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewTokenRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, token("k1", "u1", "ci")))
	later := created.Add(time.Hour)

	require.NoError(t, repo.Touch(ctx, "k1", later, "10.0.0.7"))
	require.NoError(t, repo.Revoke(ctx, "k1", later))
	require.NoError(t, repo.Revoke(ctx, "k1", later.Add(time.Hour)))
	got, err := repo.FindByID(ctx, "k1")

	require.NoError(t, err)
	require.Equal(t, later, got.LastUsedAt.UTC())
	require.Equal(t, "10.0.0.7", got.LastUsedIP)
	require.Equal(t, later, got.RevokedAt.UTC())
}
