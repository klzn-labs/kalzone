package tokens_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/stretchr/testify/require"
)

func Test_Authenticate_ReturnsTheOwnerAndToken(t *testing.T) {
	f := newFixture()
	issued, err := f.usecase.Create(ctx, ana, draftOf("read"))
	require.NoError(t, err)

	got, err := f.usecase.Authenticate(ctx, issued.Secret, "10.0.0.1")

	require.NoError(t, err)
	a := got.Actor()
	require.Equal(t, "u-ana", a.UserID)
	require.Equal(t, issued.Token.ID, a.Token.ID)
	require.True(t, a.ViaToken())
}

func Test_Authenticate_ReportsInvalidExpiredAndRevoked(t *testing.T) {
	f := newFixture()
	expiring, _ := f.usecase.Create(ctx, ana, entities.TokenDraft{Name: "short", Scopes: []string{"read"}, Teams: []string{"places"}, ExpiresInDays: 1})
	revoked, _ := f.usecase.Create(ctx, ana, draftOf("read"))
	require.NoError(t, f.usecase.Revoke(ctx, ana, revoked.Token.ID))

	_, garbage := f.usecase.Authenticate(ctx, "plat_nope", "")
	_, unknown := f.usecase.Authenticate(ctx, "plat_"+string(make([]byte, 43)), "")
	_, gone := f.usecase.Authenticate(ctx, revoked.Secret, "")
	clock.WithTime(now.Add(25 * time.Hour))
	_, late := f.usecase.Authenticate(ctx, expiring.Secret, "")

	require.ErrorIs(t, garbage, entities.ErrTokenInvalid)
	require.ErrorIs(t, unknown, entities.ErrTokenInvalid)
	require.ErrorIs(t, gone, entities.ErrTokenRevoked)
	require.ErrorIs(t, late, entities.ErrTokenExpired)
}

func Test_Authenticate_TouchesAtMostOncePerMinute(t *testing.T) {
	f := newFixture()
	issued, _ := f.usecase.Create(ctx, ana, draftOf("read"))

	for i := 0; i < 5; i++ {
		_, err := f.usecase.Authenticate(ctx, issued.Secret, "10.0.0.1")
		require.NoError(t, err)
	}
	_, _ = f.usecase.Authenticate(ctx, issued.Secret, "10.0.0.2")
	clock.WithTime(now.Add(2 * time.Minute))
	_, _ = f.usecase.Authenticate(ctx, issued.Secret, "10.0.0.2")

	require.Equal(t, 3, f.store.touched)
	require.Equal(t, "10.0.0.2", f.store.byID[issued.Token.ID].LastUsedIP)
}

func Test_ActorOf_RebuildsTheTokensActorWhileItIsValid(t *testing.T) {
	f := newFixture()
	issued, _ := f.usecase.Create(ctx, ana, draftOf("deploy", "protected"))
	revoked, _ := f.usecase.Create(ctx, ana, lasting(1, entities.TokenDraft{Name: "gone", Scopes: []string{"read"}, Teams: []string{"places"}}))
	require.NoError(t, f.usecase.Revoke(ctx, ana, revoked.Token.ID))

	a, err := f.usecase.ActorOf(ctx, issued.Token.ID)
	_, gone := f.usecase.ActorOf(ctx, revoked.Token.ID)
	clock.WithTime(now.Add(91 * 24 * time.Hour))
	_, late := f.usecase.ActorOf(ctx, issued.Token.ID)

	require.NoError(t, err)
	require.Equal(t, "u-ana", a.UserID)
	require.Equal(t, issued.Token.ID, a.Token.ID)
	require.ErrorIs(t, gone, entities.ErrTokenRevoked)
	require.ErrorIs(t, late, entities.ErrTokenExpired)
}
