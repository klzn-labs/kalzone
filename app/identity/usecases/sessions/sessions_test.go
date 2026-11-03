package sessions_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/usecases/sessions"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/demo"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

type fakeSessions struct {
	created  []entities.Session
	byHash   map[string]entities.Session
	extended map[string]time.Time
	deleted  []string
}

func (f *fakeSessions) Create(_ context.Context, s entities.Session) error {
	f.created = append(f.created, s)
	f.byHash[s.TokenHash] = s
	return nil
}

func (f *fakeSessions) DeleteExpired(_ context.Context, at time.Time) (int, error) {
	swept := 0
	for hash, s := range f.byHash {
		if !at.Before(s.ExpiresAt) {
			delete(f.byHash, hash)
			swept++
		}
	}
	return swept, nil
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
	if id == demo.GuestID {
		return entities.User{ID: id, Kind: entities.KindGuest, Login: demo.GuestLogin}, nil
	}
	user := entities.User{ID: id, Login: "ana", Admin: true}
	if id == "u-gone" {
		user.DisabledAt = &now
	}
	return user, nil
}

type fakeTeams struct{}

func (fakeTeams) VisibleTeamIDs(_ context.Context, userID string) ([]string, error) {
	return []string{"t-" + userID}, nil
}

func setup(expiresIn time.Duration) (*fakeSessions, sessions.SessionsUsecase) {
	clock.WithTime(now)
	store := &fakeSessions{
		byHash:   map[string]entities.Session{entities.HashToken("tok"): {TokenHash: entities.HashToken("tok"), CSRFToken: "csrf", UserID: "u1", ExpiresAt: now.Add(expiresIn)}},
		extended: map[string]time.Time{},
	}
	cfg := config.Config{
		Session: config.Session{TTL: 7 * 24 * time.Hour, RenewWithin: 24 * time.Hour},
		Demo:    config.Demo{Enabled: true, SessionTTL: 2 * time.Hour},
	}
	return store, sessions.NewSessionsUsecase(cfg, store, fakeUsers{}, fakeTeams{})
}

func Test_Authenticate_ReturnsTheActorAndCSRFToken(t *testing.T) {
	store, usecase := setup(72 * time.Hour)

	auth, err := usecase.Authenticate(context.Background(), "tok")

	require.NoError(t, err)
	require.Equal(t, "u1", auth.Actor().UserID)
	require.True(t, auth.Actor().Admin)
	require.Equal(t, []string{"t-u1"}, auth.Actor().Teams)
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

func Test_EnterDemo_OpensAShortGuestSessionThatNeverRenews(t *testing.T) {
	store, usecase := setup(72 * time.Hour)

	issued, err := usecase.EnterDemo(context.Background())
	require.NoError(t, err)
	clock.WithTime(now.Add(90 * time.Minute))
	auth, authErr := usecase.Authenticate(context.Background(), issued.Token)
	clock.WithTime(now.Add(2 * time.Hour))
	_, expired := usecase.Authenticate(context.Background(), issued.Token)

	require.Equal(t, now.Add(2*time.Hour), issued.ExpiresAt)
	require.Equal(t, demo.GuestID, store.created[0].UserID)
	require.NoError(t, authErr)
	require.True(t, auth.Actor().Guest)
	require.Empty(t, store.extended)
	require.ErrorIs(t, expired, apperror.New(apperror.CodeUnauthenticated, ""))
}

func Test_EnterDemo_IsRefusedWhileTheDemoIsOff(t *testing.T) {
	cfg := config.Config{Session: config.Session{TTL: time.Hour, RenewWithin: time.Minute}}
	usecase := sessions.NewSessionsUsecase(cfg, &fakeSessions{byHash: map[string]entities.Session{}}, fakeUsers{}, fakeTeams{})

	_, err := usecase.EnterDemo(context.Background())

	require.ErrorIs(t, err, apperror.NotFound(""))
}

func Test_Sweep_RemovesExpiredSessions(t *testing.T) {
	store, usecase := setup(-time.Minute)

	swept, err := usecase.Sweep(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, swept)
	require.Empty(t, store.byHash)
}

func Test_Authenticate_RefusesASessionOfADisabledUser(t *testing.T) {
	store, usecase := setup(72 * time.Hour)
	store.byHash[entities.HashToken("gone")] = entities.Session{TokenHash: entities.HashToken("gone"), UserID: "u-gone", ExpiresAt: now.Add(time.Hour)}

	_, err := usecase.Authenticate(context.Background(), "gone")

	require.ErrorIs(t, err, apperror.New(apperror.CodeUnauthenticated, ""))
}
