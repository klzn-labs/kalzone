package tokens_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/usecases/tokens"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/scope"
	"github.com/stretchr/testify/require"
)

func Test_Create_IssuesAPlatTokenAndStoresOnlyItsHash(t *testing.T) {
	f := newFixture()

	issued, err := f.usecase.Create(ctx, ana, draftOf("read", "deploy"))

	require.NoError(t, err)
	stored := f.store.byID[issued.Token.ID]
	require.Regexp(t, `^plat_[0-9A-Za-z]{43}$`, issued.Secret)
	require.Equal(t, entities.HashToken(issued.Secret), stored.Hash)
	require.Equal(t, issued.Secret[5:13], stored.Prefix)
	require.Equal(t, "u-ana", stored.OwnerID)
	require.Equal(t, []string{"t1"}, stored.TeamIDs)
	require.Equal(t, []scope.Scope{scope.Read, scope.Deploy}, stored.Scopes)
	require.Equal(t, now.Add(90*24*time.Hour), stored.ExpiresAt)
	require.Equal(t, []string{"token.created"}, f.events.actions)
}

func Test_Create_NamesAreUniquePerOwner(t *testing.T) {
	f := newFixture()
	_, err := f.usecase.Create(ctx, ana, draftOf("read"))
	require.NoError(t, err)

	_, again := f.usecase.Create(ctx, ana, draftOf("read"))
	_, otherOwner := f.usecase.Create(ctx, ops, draftOf("read"))

	require.ErrorIs(t, again, apperror.Conflict(""))
	require.NoError(t, otherOwner)
}

func Test_Create_ARevokedTokenFreesItsName(t *testing.T) {
	f := newFixture()
	first, err := f.usecase.Create(ctx, ana, draftOf("read"))
	require.NoError(t, err)
	require.NoError(t, f.usecase.Revoke(ctx, ana, first.Token.ID))

	second, err := f.usecase.Create(ctx, ana, draftOf("read"))

	require.NoError(t, err)
	require.NotEqual(t, first.Token.ID, second.Token.ID)
}

func Test_Create_RejectsUnknownTeams(t *testing.T) {
	d := draftOf("read")
	d.Teams = []string{"ghosts"}

	_, err := newFixture().usecase.Create(ctx, ana, d)

	require.ErrorIs(t, err, apperror.NotFound(""))
}

func Test_Create_BypassOnlyFromAnAdminSession(t *testing.T) {
	f := newFixture()
	d := draftOf("deploy", "protected")
	d.Bypass = true
	adminToken := agentOf(ops, scope.All, []string{"t1"}, 365)
	adminToken.Token.Bypass = true

	_, member := f.usecase.Create(ctx, ana, d)
	_, viaToken := f.usecase.Create(ctx, adminToken, d)
	issued, admin := f.usecase.Create(ctx, ops, d)

	require.ErrorIs(t, member, apperror.Forbidden(""))
	require.ErrorIs(t, viaToken, apperror.Forbidden(""))
	require.NoError(t, admin)
	require.True(t, issued.Token.Bypass)
}

func Test_Create_FromATokenNeverEscalates(t *testing.T) {
	f := newFixture()
	parent := agentOf(ana, []scope.Scope{scope.TokensAdmin, scope.Read}, []string{"t1"}, 30)
	otherTeam := draftOf("read")
	otherTeam.Teams = []string{"reviews"}

	_, scopeErr := f.usecase.Create(ctx, parent, lasting(30, draftOf("read", "deploy")))
	_, teamErr := f.usecase.Create(ctx, parent, lasting(30, otherTeam))
	child, err := f.usecase.Create(ctx, parent, lasting(31, draftOf("read")))

	require.ErrorIs(t, scopeErr, apperror.Forbidden(""))
	require.ErrorIs(t, teamErr, apperror.Forbidden(""))
	require.NoError(t, err)
	require.Equal(t, parent.Token.ExpiresAt, child.Token.ExpiresAt)
	require.Equal(t, "parent", f.store.byID[child.Token.ID].CreatedByTokenID)
}

func Test_Create_FromATokenNeedsTokensAdmin(t *testing.T) {
	_, err := newFixture().usecase.Create(ctx, agentOf(ana, []scope.Scope{scope.Read}, []string{"t1"}, 30), draftOf("read"))

	require.ErrorIs(t, err, apperror.MissingScope(""))
}

func Test_Revoke_OnlyTheOwnerOrAnAdminSession(t *testing.T) {
	f := newFixture()
	issued, err := f.usecase.Create(ctx, ana, draftOf("read"))
	require.NoError(t, err)
	adminToken := agentOf(ops, scope.All, []string{"t1"}, 30)

	stranger := f.usecase.Revoke(ctx, adminToken, issued.Token.ID)
	require.ErrorIs(t, stranger, apperror.NotFound(""))
	require.NoError(t, f.usecase.Revoke(ctx, ops, issued.Token.ID))
	require.NoError(t, f.usecase.Revoke(ctx, ana, issued.Token.ID))

	require.NotNil(t, f.store.byID[issued.Token.ID].RevokedAt)
	require.Equal(t, []string{"token.created", "token.revoked"}, f.events.actions)
}

func Test_ListAll_OnlyForAnAdminSession(t *testing.T) {
	f := newFixture()

	_, member := f.usecase.ListAll(ctx, ana)
	_, viaToken := f.usecase.ListAll(ctx, agentOf(ops, scope.All, []string{"t1"}, 30))
	_, admin := f.usecase.ListAll(ctx, ops)

	require.ErrorIs(t, member, apperror.Forbidden(""))
	require.ErrorIs(t, viaToken, apperror.Forbidden(""))
	require.NoError(t, admin)
}

func Test_Whoami_NamesTheTeams(t *testing.T) {
	who, err := newFixture().usecase.Whoami(ctx, agentOf(ana, []scope.Scope{scope.Read}, []string{"t1"}, 30))

	require.NoError(t, err)
	require.Equal(t, "ana", who.User.Login)
	require.Equal(t, []string{"places"}, who.Teams)
	require.Equal(t, "parent", who.Token.Name)
}

func Test_ListAndCreate_NameTheTeamsBySlug(t *testing.T) {
	f := newFixture()

	issued, err := f.usecase.Create(ctx, ana, draftOf("read"))
	require.NoError(t, err)
	mine, err := f.usecase.List(ctx, ana)
	require.NoError(t, err)
	all, err := f.usecase.ListAll(ctx, ops)
	require.NoError(t, err)

	require.Equal(t, []string{"places"}, issued.Token.TeamSlugs)
	require.Equal(t, []string{"places"}, mine[0].TeamSlugs)
	require.Equal(t, []string{"places"}, all[0].TeamSlugs)
}

type unnamedTeams struct{ teams }

func (unnamedTeams) SlugsByIDs(context.Context, []string) ([]string, error) {
	return nil, errors.New("teams unavailable")
}

func Test_Create_StoresNothingWhenTheTeamsCannotBeNamed(t *testing.T) {
	f := newFixture()
	broken := tokens.NewTokensUsecase(f.store, users{}, unnamedTeams{teams{"places": "t1"}}, allow{}, f.events)

	issued, err := broken.Create(ctx, ana, draftOf("read"))

	require.Error(t, err)
	require.Equal(t, entities.IssuedToken{}, issued)
	require.Empty(t, f.store.byID)
	require.Empty(t, f.events.actions)
}
