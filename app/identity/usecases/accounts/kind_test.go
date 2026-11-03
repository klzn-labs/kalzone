package accounts_test

import (
	"errors"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/usecases/accounts"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func plantLink(f *fixture, kind entities.LinkKind, expires time.Time, used bool) string {
	token := string(kind) + "-token"
	link := entities.AccountLink{ID: string(kind), Kind: kind, TokenHash: entities.HashToken(token), ExpiresAt: expires}
	if used {
		link.UsedAt = &now
	}
	f.links.byHash[link.TokenHash] = link
	return token
}

func Test_LinkKindNamesTheKindOfALiveLink(t *testing.T) {
	f := newFixture(passwordConfig())
	for _, kind := range []entities.LinkKind{entities.LinkSetup, entities.LinkInvite, entities.LinkReset} {
		got, err := f.usecase.LinkKind(ctx, plantLink(f, kind, now.Add(time.Hour), false))
		require.NoError(t, err)
		require.Equal(t, kind, got)
	}
	for _, link := range f.links.byHash {
		require.Nil(t, link.UsedAt)
	}
}

func Test_LinkKindRefusesDeadLinksWithOneMessage(t *testing.T) {
	f := newFixture(passwordConfig())
	tokens := []string{"unknown", plantLink(f, entities.LinkInvite, now.Add(-time.Minute), false), plantLink(f, entities.LinkReset, now.Add(time.Hour), true)}
	for _, token := range tokens {
		_, err := f.usecase.LinkKind(ctx, token)
		require.ErrorIs(t, err, apperror.NotFound(""))
		require.EqualError(t, err, "This link is no longer valid.")
	}
}

func Test_LinkKindIsOffInGitHubMode(t *testing.T) {
	f := newFixture(githubConfig())
	_, err := f.usecase.LinkKind(ctx, plantLink(f, entities.LinkSetup, now.Add(time.Hour), false))
	require.Equal(t, apperror.CodeGitHubMode, apperror.CodeOf(err))
}

func Test_CompleteReportsAnOwnerCreatedWithoutAToken(t *testing.T) {
	f := newFixture(passwordConfig())
	f.tokens.err = errors.New("token store down")
	in := accounts.SetupInput{Username: "ana", Password: password, Confirm: password, Bypass: true}
	_, err := f.usecase.Complete(ctx, setupToken(t, f), in)
	var unfinished accounts.SetupUnfinishedError
	require.ErrorAs(t, err, &unfinished)
	require.Equal(t, "ana", f.store.owner.Login)
}
