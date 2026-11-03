package accounts_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/usecases/accounts"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func invite(t *testing.T, f *peopleFixture, a actor.Actor, team, role string) accounts.Invite {
	t.Helper()
	created, err := f.invites.Create(ctx, a, team, role)
	require.NoError(t, err)
	return created
}

func Test_MaintainerInvitesToOwnTeamOnly(t *testing.T) {
	f := newPeopleFixture(passwordConfig())
	invite(t, f, maintainer, "places", "member")
	invite(t, f, admin, "reviews", "member")

	_, err := f.invites.Create(ctx, maintainer, "reviews", "member")
	require.ErrorIs(t, err, apperror.Forbidden(""))
	_, err = f.invites.Create(ctx, member, "places", "member")
	require.ErrorIs(t, err, apperror.Forbidden(""))
	_, err = f.invites.List(ctx, member, "places")
	require.ErrorIs(t, err, apperror.Forbidden(""))
	_, err = f.invites.List(ctx, maintainer, "reviews")
	require.ErrorIs(t, err, apperror.Forbidden(""))
	require.ErrorIs(t, f.invites.Revoke(ctx, member, "places", f.links.all()[0].ID), apperror.Forbidden(""))

	listed, err := f.invites.List(ctx, maintainer, "places")
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "t-places", listed[0].TeamID)
}

func Test_InviteFixesTeamAndRole(t *testing.T) {
	f := newPeopleFixture(passwordConfig())
	created := invite(t, f, maintainer, "places", "maintainer")
	requireMaintainerInvite(t, created)

	issued, err := f.invites.Accept(ctx, tokenOf(t, created.URL), " Bea ", password, password, false)
	require.NoError(t, err)
	requireAcceptedAsMaintainer(t, f, created.Link)
	requireSessionFor(t, f, f.store.accepted[0].user, issued.Token)

	_, err = f.invites.Accept(ctx, tokenOf(t, created.URL), "cid", password, password, false)
	require.Equal(t, errGone, err)
}

func requireMaintainerInvite(t *testing.T, created accounts.Invite) {
	t.Helper()
	link := created.Link
	require.Equal(t, entities.LinkInvite, link.Kind)
	require.Equal(t, "t-places", link.TeamID)
	require.Equal(t, "maintainer", link.Role)
	require.Equal(t, "u-maint", link.CreatedBy)
	require.Equal(t, now.Add(7*24*time.Hour), link.ExpiresAt)
	require.Equal(t, entities.HashToken(tokenOf(t, created.URL)), link.TokenHash)
}

func requireAcceptedAsMaintainer(t *testing.T, f *peopleFixture, link entities.AccountLink) {
	t.Helper()
	require.Len(t, f.store.accepted, 1)
	got := f.store.accepted[0]
	require.Equal(t, link.ID, got.link.ID)
	require.Equal(t, "t-places", got.link.TeamID)
	require.Equal(t, "maintainer", got.link.Role)
	require.Equal(t, "bea", got.user.Login)
	require.Equal(t, entities.KindPerson, got.user.Kind)
	require.False(t, got.user.Admin)
	require.True(t, entities.VerifyPassword(got.user.PasswordHash, password))
}

func requireSessionFor(t *testing.T, f *peopleFixture, user entities.User, token string) {
	t.Helper()
	require.Len(t, f.sessions.created, 1)
	require.Equal(t, user.ID, f.sessions.created[0].UserID)
	require.Equal(t, entities.HashToken(token), f.sessions.created[0].TokenHash)
}

func Test_InviteRefusesAnUnknownRole(t *testing.T) {
	f := newPeopleFixture(passwordConfig())
	_, err := f.invites.Create(ctx, admin, "places", "owner")
	require.ErrorIs(t, err, apperror.Invalid(""))
	require.Empty(t, f.links.all())
}

func Test_AcceptRefusesSignedInPeopleAndOtherLinks(t *testing.T) {
	f := newPeopleFixture(passwordConfig())
	f.users["ana"] = entities.User{ID: "u-ana", Login: "ana", PasswordHash: "h"}
	created := invite(t, f, admin, "places", "member")
	_, err := f.invites.Accept(ctx, tokenOf(t, created.URL), "bea", password, password, true)
	require.ErrorIs(t, err, apperror.Conflict(""))
	require.EqualError(t, err, "Sign out to accept this invite")

	reset, err := f.people.IssueReset(ctx, admin, "ana")
	require.NoError(t, err)
	_, err = f.invites.Accept(ctx, tokenOf(t, reset), "bea", password, password, false)
	require.Equal(t, errGone, err)

	_, err = f.invites.Accept(ctx, tokenOf(t, created.URL), "bea", password, "different password", false)
	require.ErrorIs(t, err, apperror.Invalid(""))
	require.Empty(t, f.store.accepted)
	require.Empty(t, f.sessions.created)
}

func Test_RevokeOnlyTheTeamsOwnInvites(t *testing.T) {
	f := newPeopleFixture(passwordConfig())
	mine := invite(t, f, maintainer, "places", "member")
	theirs := invite(t, f, admin, "reviews", "member")

	err := f.invites.Revoke(ctx, maintainer, "places", theirs.Link.ID)
	require.ErrorIs(t, err, apperror.NotFound(""))
	err = f.invites.Revoke(ctx, admin, "places", theirs.Link.ID)
	require.ErrorIs(t, err, apperror.NotFound(""))
	require.Nil(t, f.links.byHash[theirs.Link.TokenHash].RevokedAt)

	require.NoError(t, f.invites.Revoke(ctx, maintainer, "places", mine.Link.ID))
	listed, err := f.invites.List(ctx, maintainer, "places")
	require.NoError(t, err)
	require.Empty(t, listed)
	_, err = f.invites.Accept(ctx, tokenOf(t, mine.URL), "bea", password, password, false)
	require.Equal(t, errGone, err)
}

func Test_GitHubModeRefusesInvitesAndResets(t *testing.T) {
	f := newPeopleFixture(githubConfig())
	f.users["ana"] = entities.User{ID: "u-ana", Login: "ana", PasswordHash: "h"}
	f.users["bob"] = entities.User{ID: "u-bob", Login: "bob", GitHubID: 9}
	f.users["admin"] = entities.User{ID: "u-admin", Login: "admin", Admin: true, GitHubID: 1}
	_, createErr := f.invites.Create(ctx, admin, "places", "member")
	_, listErr := f.invites.List(ctx, admin, "places")
	_, acceptErr := f.invites.Accept(ctx, "any", "bea", password, password, false)
	_, issueErr := f.people.IssueReset(ctx, admin, "ana")
	errs := []error{
		createErr, listErr, acceptErr, issueErr,
		f.invites.Revoke(ctx, admin, "places", "any"),
		f.people.UseReset(ctx, "any", password, password, ip),
		f.people.ChangePassword(ctx, actor.Actor{UserID: "u-ana", Login: "ana"}, password, password, password, "keep"),
	}
	for _, err := range errs {
		require.Equal(t, apperror.CodeGitHubMode, apperror.CodeOf(err), "%v", err)
	}
	require.Empty(t, f.links.all())
	require.NoError(t, f.people.Remove(ctx, admin, "bob"))
}
