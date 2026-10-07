package sessions_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/identity/entities"
	"github.com/adrianozp/platform/app/identity/usecases/sessions"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

type fakeSessions struct {
	byHash   map[string]entities.Session
	extended map[string]time.Time
	deleted  []string
}

func (f *fakeSessions) Find(_ context.Context, hash string) (entities.Session, error) {
	s, ok := f.byHash[hash]
	if !ok {
		return entities.Session{}, apperror.New(apperror.CodeUnauthenticated, "no session")
	}
	return s, nil
}

func (f *fakeSessions) Extend(_ context.Context, hash string, at time.Time) error {
	f.extended[hash] = at
	return nil
}

func (f *fakeSessions) Delete(_ context.Context, hash string) error {
	f.deleted = append(f.deleted, hash)
	return nil
}

type fakeUsers struct{}

func (fakeUsers) FindByID(_ context.Context, id string) (entities.User, error) {
	return entities.User{ID: id, Login: "ana", Admin: true}, nil
}

func setup(expiresIn time.Duration) (*fakeSessions, sessions.SessionsUsecase) {
	clock.WithTime(now)
	store := &fakeSessions{
		byHash:   map[string]entities.Session{entities.HashToken("tok"): {TokenHash: entities.HashToken("tok"), CSRFToken: "csrf", UserID: "u1", ExpiresAt: now.Add(expiresIn)}},
		extended: map[string]time.Time{},
	}
	cfg := config.Config{Session: config.Session{TTL: 7 * 24 * time.Hour, RenewWithin: 24 * time.Hour}}
	return store, sessions.NewSessionsUsecase(cfg, store, fakeUsers{})
}

func Test_Authenticate_ReturnsTheActorAndCSRFToken(t *testing.T) {
	store, usecase := setup(72 * time.Hour)

	auth, err := usecase.Authenticate(context.Background(), "tok")

	require.NoError(t, err)
	require.Equal(t, "u1", auth.Actor().UserID)
	require.True(t, auth.Actor().Admin)
	require.Equal(t, "csrf", auth.CSRFToken)
	require.Empty(t, store.extended)
}

func Test_Authenticate_RenewsASessionCloseToExpiry(t *testing.T) {
	store, usecase := setup(time.Hour)

	_, err := usecase.Authenticate(context.Background(), "tok")

	require.NoError(t, err)
	require.Equal(t, now.Add(7*24*time.Hour), store.extended[entities.HashToken("tok")])
}

func Test_Authenticate_RejectsExpiredUnknownAndEmptyTokens(t *testing.T) {
	_, expired := setup(-time.Second)
	_, live := setup(time.Hour)
	ctx := context.Background()
	unauthenticated := apperror.New(apperror.CodeUnauthenticated, "")

	_, expiredErr := expired.Authenticate(ctx, "tok")
	_, unknownErr := live.Authenticate(ctx, "other")
	_, emptyErr := live.Authenticate(ctx, "")

	require.ErrorIs(t, expiredErr, unauthenticated)
	require.ErrorIs(t, unknownErr, unauthenticated)
	require.ErrorIs(t, emptyErr, unauthenticated)
}

func Test_SignOut_DeletesTheSessionByHash(t *testing.T) {
	store, usecase := setup(time.Hour)

	require.NoError(t, usecase.SignOut(context.Background(), "tok"))

	require.Equal(t, []string{entities.HashToken("tok")}, store.deleted)
}
