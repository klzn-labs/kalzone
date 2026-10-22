package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/stretchr/testify/require"
)

func Test_Users_PeopleWithoutGitHubKeepTheirKind(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewUserRepository(testdb.Open(t))
	require.NoError(t, repo.Save(ctx, entities.User{ID: "u1", GitHubID: 42, Login: "ana", CreatedAt: created, LastLoginAt: created}))
	require.NoError(t, repo.Save(ctx, entities.User{ID: "g1", Kind: entities.KindGuest, Login: "guest.demo", CreatedAt: created, LastLoginAt: created}))
	require.NoError(t, repo.Save(ctx, entities.User{ID: "p1", Kind: entities.KindPersona, Login: "mia.demo", CreatedAt: created, LastLoginAt: created}))

	ana, err := repo.FindByGitHubID(ctx, 42)
	require.NoError(t, err)
	guest, err := repo.FindByID(ctx, "g1")
	require.NoError(t, err)
	persona, err := repo.FindByLogin(ctx, "mia.demo")
	require.NoError(t, err)

	require.Equal(t, entities.KindPerson, ana.Kind)
	require.True(t, guest.Guest())
	require.Zero(t, guest.GitHubID)
	require.Equal(t, entities.KindPersona, persona.Kind)
}

func Test_Sessions_DeleteExpiredKeepsLiveOnes(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewSessionRepository(testdb.Open(t))
	require.NoError(t, repo.Create(ctx, entities.Session{TokenHash: "old", UserID: "u1", CreatedAt: created, ExpiresAt: created.Add(time.Hour)}))
	require.NoError(t, repo.Create(ctx, entities.Session{TokenHash: "new", UserID: "u1", CreatedAt: created, ExpiresAt: created.Add(3 * time.Hour)}))

	swept, err := repo.DeleteExpired(ctx, created.Add(2*time.Hour))
	_, live := repo.Find(ctx, "new")

	require.NoError(t, err)
	require.Equal(t, 1, swept)
	require.NoError(t, live)
}
