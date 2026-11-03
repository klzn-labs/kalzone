package accounts_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/usecases/accounts"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/klzn-labs/kalzone/pkg/scope"
	"github.com/stretchr/testify/require"
)

func tokenOf(t *testing.T, url string) string {
	t.Helper()
	token, ok := strings.CutPrefix(url, baseURL+"/join/")
	require.True(t, ok, url)
	return token
}

func setupToken(t *testing.T, f *fixture) string {
	t.Helper()
	url, err := f.usecase.NewSetupLink(ctx)
	require.NoError(t, err)
	return tokenOf(t, url)
}

func Test_EnsureSetupLinkOnlyWithoutPeople(t *testing.T) {
	f := newFixture(passwordConfig())
	f.store.people = 1
	require.NoError(t, f.usecase.EnsureSetupLink(ctx))
	require.Empty(t, f.links.all())
	require.Empty(t, f.logger.lines)

	f.store.people = 0
	require.NoError(t, f.usecase.EnsureSetupLink(ctx))
	require.Len(t, f.links.all(), 1)
	require.Len(t, f.logger.lines, 1)
	line := f.logger.lines[0]
	require.True(t, strings.HasPrefix(line, "WARN setup: open "+baseURL+"/join/"), line)
	require.True(t, strings.HasSuffix(line, " to create the owner"), line)
	token := tokenOf(t, strings.Fields(line)[3])
	link := f.links.all()[0]
	require.Equal(t, entities.HashToken(token), link.TokenHash)
	require.Equal(t, entities.LinkSetup, link.Kind)
	require.Equal(t, now.Add(24*time.Hour), link.ExpiresAt)
}

func Test_NewSetupLinkRefusesOnceAnOwnerExists(t *testing.T) {
	f := newFixture(passwordConfig())
	f.store.people = 1
	_, err := f.usecase.NewSetupLink(ctx)
	require.ErrorIs(t, err, apperror.Conflict(""))
	require.EqualError(t, err, "this Kalzone already has an owner")
	require.Empty(t, f.links.all())
}

func Test_EnsureSetupLinkRevokesTheEarlierOne(t *testing.T) {
	f := newFixture(passwordConfig())
	require.NoError(t, f.usecase.EnsureSetupLink(ctx))
	require.NoError(t, f.usecase.EnsureSetupLink(ctx))
	all := f.links.all()
	require.Len(t, all, 2)
	require.NotNil(t, all[0].RevokedAt)
	require.Nil(t, all[1].RevokedAt)
	require.Equal(t, []entities.LinkKind{entities.LinkSetup, entities.LinkSetup}, f.links.revoked)
}

func Test_GitHubModeMakesNoSetupLink(t *testing.T) {
	f := newFixture(githubConfig())
	require.NoError(t, f.usecase.EnsureSetupLink(ctx))
	require.Empty(t, f.links.all())
	require.Empty(t, f.logger.lines)
	_, err := f.usecase.NewSetupLink(ctx)
	require.Equal(t, apperror.CodeGitHubMode, apperror.CodeOf(err))
}

func Test_ModeFollowsGitHubCredentials(t *testing.T) {
	cfg := passwordConfig()
	cfg.GitHub.ClientID = "id"
	require.False(t, accounts.NewMode(cfg).GitHub)
	require.NoError(t, accounts.NewMode(cfg).RequirePasswords())
	err := accounts.NewMode(githubConfig()).RequirePasswords()
	require.Equal(t, apperror.CodeGitHubMode, apperror.CodeOf(err))
	var appErr *apperror.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, "this Kalzone signs people in with GitHub; add them to the team after they sign in", appErr.Hint)
}

func Test_CompleteCreatesAnAdminOwnerAndAgentToken(t *testing.T) {
	f := newFixture(passwordConfig())
	token := setupToken(t, f)
	in := accounts.SetupInput{Username: " Ana ", Password: password, Confirm: password, Bypass: true}
	result, err := f.usecase.Complete(ctx, token, in)
	require.NoError(t, err)
	requireOwner(t, f)
	require.Len(t, f.sessions.created, 1)
	require.Equal(t, f.store.owner.ID, f.sessions.created[0].UserID)
	require.Equal(t, entities.HashToken(result.Session.Token), f.sessions.created[0].TokenHash)
	requireAgentToken(t, f)
	require.Equal(t, "kz_agent_secret", result.AgentToken)
	require.Equal(t, `claude mcp add --transport http kalzone https://kalzone.test/mcp --header "Authorization: Bearer kz_agent_secret"`, result.MCPCommand)
	requireSetupEvent(t, f, f.store.owner)
}

func requireOwner(t *testing.T, f *fixture) {
	t.Helper()
	owner := f.store.owner
	require.True(t, owner.Admin)
	require.Equal(t, "ana", owner.Login)
	require.Equal(t, entities.KindPerson, owner.Kind)
	require.True(t, entities.VerifyPassword(owner.PasswordHash, password))
	require.Equal(t, entities.NewTeam{ID: f.store.team.ID, Slug: "default", Name: "Default", CreatedAt: now}, f.store.team)
}

func requireAgentToken(t *testing.T, f *fixture) {
	t.Helper()
	require.Equal(t, f.store.owner.ID, f.tokens.actor.UserID)
	require.True(t, f.tokens.actor.SessionAdmin())
	require.Equal(t, []string{f.store.team.ID}, f.tokens.actor.Teams)
	draft := f.tokens.draft
	require.Equal(t, "agent", draft.Name)
	require.Len(t, draft.Scopes, len(scope.All))
	for _, s := range scope.All {
		require.Contains(t, draft.Scopes, string(s))
	}
	require.Equal(t, []string{"default"}, draft.Teams)
	require.Equal(t, 90, draft.ExpiresInDays)
	require.True(t, draft.Bypass)
}

func requireSetupEvent(t *testing.T, f *fixture, owner entities.User) {
	t.Helper()
	require.Len(t, f.publisher.events, 1)
	event := f.publisher.events[0]
	require.Equal(t, "setup.completed", event.Action)
	require.Equal(t, owner.ID, event.ActorID)
	require.Equal(t, events.TargetUser, event.TargetKind)
	require.Equal(t, "ana", event.TargetName)
	dump := fmt.Sprintf("%+v", event)
	for _, secret := range []string{password, owner.PasswordHash, "kz_agent_secret"} {
		require.NotContains(t, dump, secret)
	}
}

func Test_CompleteHonoursBypassOff(t *testing.T) {
	f := newFixture(passwordConfig())
	in := accounts.SetupInput{Username: "ana", Password: password, Confirm: password}
	_, err := f.usecase.Complete(ctx, setupToken(t, f), in)
	require.NoError(t, err)
	require.False(t, f.tokens.draft.Bypass)
}

func Test_CompleteRefusesMismatchedPasswords(t *testing.T) {
	f := newFixture(passwordConfig())
	in := accounts.SetupInput{Username: "ana", Password: password, Confirm: password + "!"}
	_, err := f.usecase.Complete(ctx, setupToken(t, f), in)
	require.ErrorIs(t, err, apperror.Invalid(""))
	require.Empty(t, f.store.owner.ID)
	require.Empty(t, f.sessions.created)
	require.Empty(t, f.publisher.events)
}

func Test_CompleteRefusesAUsedLink(t *testing.T) {
	f := newFixture(passwordConfig())
	token := setupToken(t, f)
	in := accounts.SetupInput{Username: "ana", Password: password, Confirm: password}
	_, err := f.usecase.Complete(ctx, token, in)
	require.NoError(t, err)
	_, err = f.usecase.Complete(ctx, token, in)
	require.EqualError(t, err, "This link is no longer valid.")
	require.Len(t, f.sessions.created, 1)
}

func Test_ResetLinkRefusesDisabledAndGitHubUsers(t *testing.T) {
	f := newFixture(passwordConfig())
	f.users["gone"] = entities.User{ID: "u-gone", Login: "gone", PasswordHash: "h", DisabledAt: &now}
	f.users["octo"] = entities.User{ID: "u-octo", Login: "octo", GitHubID: 7}
	f.users["ana"] = entities.User{ID: "u-ana", Login: "ana", PasswordHash: "h"}
	for _, login := range []string{"gone", "octo"} {
		_, err := f.usecase.NewResetLinkFor(ctx, login)
		require.ErrorIs(t, err, apperror.Conflict(""))
		require.EqualError(t, err, "reset links are only for active password accounts")
	}
	require.Empty(t, f.links.all())

	url, err := f.usecase.NewResetLinkFor(ctx, "Ana")
	require.NoError(t, err)
	link := f.links.all()[0]
	require.Equal(t, entities.HashToken(tokenOf(t, url)), link.TokenHash)
	require.Equal(t, entities.LinkReset, link.Kind)
	require.Equal(t, "u-ana", link.UserID)
}

func Test_CompleteRefusesOnceAnOwnerExists(t *testing.T) {
	f := newFixture(passwordConfig())
	token := setupToken(t, f)
	f.store.people = 1
	in := accounts.SetupInput{Username: "ana", Password: password, Confirm: password}
	_, err := f.usecase.Complete(ctx, token, in)
	require.ErrorIs(t, err, apperror.Conflict(""))
	require.EqualError(t, err, "this Kalzone already has an owner")
	require.Empty(t, f.store.owner.ID)
	require.Nil(t, f.links.all()[0].UsedAt)
	require.Empty(t, f.sessions.created)
	require.Empty(t, f.publisher.events)
}

func Test_EnsureSetupLinkSurfacesOtherConflicts(t *testing.T) {
	f := newFixture(passwordConfig())
	f.links.createErr = apperror.Conflict("duplicate link")
	require.EqualError(t, f.usecase.EnsureSetupLink(ctx), "duplicate link")
	require.Empty(t, f.logger.lines)
}

func Test_GitHubModeRefusesSetupAndResets(t *testing.T) {
	f := newFixture(githubConfig())
	f.users["ana"] = entities.User{ID: "u-ana", Login: "ana", PasswordHash: "h"}
	in := accounts.SetupInput{Username: "ana", Password: password, Confirm: password}
	_, err := f.usecase.Complete(ctx, "any", in)
	require.Equal(t, apperror.CodeGitHubMode, apperror.CodeOf(err))
	_, err = f.usecase.NewResetLinkFor(ctx, "ana")
	require.Equal(t, apperror.CodeGitHubMode, apperror.CodeOf(err))
	require.Empty(t, f.links.all())
}

func Test_ResetLinkForAnUnknownLoginIsNotFound(t *testing.T) {
	f := newFixture(passwordConfig())
	_, err := f.usecase.NewResetLinkFor(ctx, "nobody")
	require.ErrorIs(t, err, apperror.NotFound(""))
	require.Empty(t, f.links.all())
}

func Test_ResetLinkIsAudited(t *testing.T) {
	f := newFixture(passwordConfig())
	f.users["ana"] = entities.User{ID: "u-ana", Login: "ana", PasswordHash: "h"}
	url, err := f.usecase.NewResetLinkFor(ctx, "ana")
	require.NoError(t, err)
	require.Len(t, f.publisher.events, 1)
	event := f.publisher.events[0]
	require.Equal(t, "user.password_reset_issued", event.Action)
	require.Equal(t, events.TargetUser, event.TargetKind)
	require.Equal(t, "u-ana", event.TargetID)
	require.Equal(t, "ana", event.TargetName)
	dump := fmt.Sprintf("%+v", event)
	require.NotContains(t, dump, tokenOf(t, url))
	require.NotContains(t, dump, f.links.all()[0].TokenHash)
}

func Test_ResetLinkRevokesTheUsersEarlierOne(t *testing.T) {
	f := newFixture(passwordConfig())
	f.users["ana"] = entities.User{ID: "u-ana", Login: "ana", PasswordHash: "h"}
	_, err := f.usecase.NewResetLinkFor(ctx, "ana")
	require.NoError(t, err)
	_, err = f.usecase.NewResetLinkFor(ctx, "ana")
	require.NoError(t, err)
	all := f.links.all()
	require.Len(t, all, 2)
	require.NotNil(t, all[0].RevokedAt)
	require.Nil(t, all[1].RevokedAt)
}
