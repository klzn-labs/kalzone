package accounts_test

import (
	"encoding/json"
	"testing"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/scope"
	"github.com/stretchr/testify/require"
)

const newPassword = "a brand new passphrase"

func hashed(t *testing.T, plain string) string {
	t.Helper()
	hash, err := entities.HashPassword(plain)
	require.NoError(t, err)
	return hash
}

func withPeople(t *testing.T, f *peopleFixture) {
	t.Helper()
	disabledAt := now
	f.users["admin"] = entities.User{ID: "u-admin", Login: "admin", Admin: true, PasswordHash: "h"}
	f.users["ana"] = entities.User{ID: "u-ana", Login: "ana", PasswordHash: hashed(t, password)}
	f.users["gone"] = entities.User{ID: "u-gone", Login: "gone", Admin: true, PasswordHash: "h", DisabledAt: &disabledAt}
	f.users["octo"] = entities.User{ID: "u-octo", Login: "octo", GitHubID: 7}
}

func Test_RemoveKeepsTheLastAdmin(t *testing.T) {
	f := newPeopleFixture(passwordConfig())
	withPeople(t, f)

	err := f.people.Remove(ctx, admin, "admin")
	require.ErrorIs(t, err, apperror.Conflict(""))
	require.EqualError(t, err, "keep at least one admin")
	require.Empty(t, f.store.disabled)

	requireRemoveForbidden(t, f)

	require.NoError(t, f.people.Remove(ctx, admin, "Octo"))
	require.Equal(t, []string{"u-octo"}, f.store.disabled)
	require.ErrorIs(t, f.people.Remove(ctx, admin, "octo"), apperror.Conflict(""))

	promote(f, "ana")
	require.NoError(t, f.people.Remove(ctx, admin, "admin"))
	require.Equal(t, []string{"u-octo", "u-admin"}, f.store.disabled)
	require.Equal(t, "user.removed", f.publisher.events[len(f.publisher.events)-1].Action)
}

func requireRemoveForbidden(t *testing.T, f *peopleFixture) {
	t.Helper()
	require.ErrorIs(t, f.people.Remove(ctx, maintainer, "ana"), apperror.Forbidden(""))
	viaToken := admin
	viaToken.Token = actor.Token{ID: "tok", Scopes: scope.All}
	require.ErrorIs(t, f.people.Remove(ctx, viaToken, "ana"), apperror.Forbidden(""))
}

func promote(f *peopleFixture, login string) {
	u := f.users[login]
	u.Admin = true
	f.users[login] = u
}

func Test_ResetLinkOnlyForActivePasswordUsers(t *testing.T) {
	f := newPeopleFixture(passwordConfig())
	withPeople(t, f)
	requireResetRefused(t, f)

	first, second := issueTwoResets(t, f)
	require.Equal(t, errGone, f.people.UseReset(ctx, tokenOf(t, first), newPassword, newPassword, ip))
	link := f.links.byHash[entities.HashToken(tokenOf(t, second))]
	require.Equal(t, "u-ana", link.UserID)
	require.Equal(t, "u-admin", link.CreatedBy)

	err := f.people.UseReset(ctx, tokenOf(t, second), newPassword, "something else", ip)
	require.ErrorIs(t, err, apperror.Invalid(""))
	require.NoError(t, f.people.UseReset(ctx, tokenOf(t, second), newPassword, newPassword, ip))
	require.True(t, entities.VerifyPassword(f.store.resets["u-ana"], newPassword))
	require.Equal(t, []string{"ana|" + ip}, f.cleared.calls)
	require.Equal(t, errGone, f.people.UseReset(ctx, tokenOf(t, second), newPassword, newPassword, ip))
}

func requireResetRefused(t *testing.T, f *peopleFixture) {
	t.Helper()
	for _, login := range []string{"gone", "octo"} {
		_, err := f.people.IssueReset(ctx, admin, login)
		require.ErrorIs(t, err, apperror.Conflict(""), login)
	}
	_, err := f.people.IssueReset(ctx, maintainer, "ana")
	require.ErrorIs(t, err, apperror.Forbidden(""))
	require.Empty(t, f.links.all())
}

func issueTwoResets(t *testing.T, f *peopleFixture) (string, string) {
	t.Helper()
	first, err := f.people.IssueReset(ctx, admin, "ana")
	require.NoError(t, err)
	second, err := f.people.IssueReset(ctx, admin, "Ana")
	require.NoError(t, err)
	return first, second
}

func Test_UseResetRefusesInviteLinks(t *testing.T) {
	f := newPeopleFixture(passwordConfig())
	created := invite(t, f, admin, "places", "member")
	require.Equal(t, errGone, f.people.UseReset(ctx, tokenOf(t, created.URL), newPassword, newPassword, ip))
	require.Empty(t, f.store.resets)
}

func Test_ChangePasswordNeedsTheCurrentOne(t *testing.T) {
	f := newPeopleFixture(passwordConfig())
	withPeople(t, f)
	ana := actor.Actor{UserID: "u-ana", Login: "ana"}

	err := f.people.ChangePassword(ctx, ana, "not the password", newPassword, newPassword, "keep")
	require.ErrorIs(t, err, apperror.Invalid(""))
	require.EqualError(t, err, "the current password is wrong")
	err = f.people.ChangePassword(ctx, ana, password, newPassword, "mismatch", "keep")
	require.ErrorIs(t, err, apperror.Invalid(""))
	octo := actor.Actor{UserID: "u-octo", Login: "octo"}
	require.ErrorIs(t, f.people.ChangePassword(ctx, octo, password, newPassword, newPassword, "keep"), apperror.Conflict(""))
	viaToken := ana
	viaToken.Token = actor.Token{ID: "tok", Scopes: scope.All}
	require.ErrorIs(t, f.people.ChangePassword(ctx, viaToken, password, newPassword, newPassword, "keep"), apperror.Forbidden(""))
	require.Empty(t, f.store.changed)

	require.NoError(t, f.people.ChangePassword(ctx, ana, password, newPassword, newPassword, "keep"))
	require.True(t, entities.VerifyPassword(f.users["ana"].PasswordHash, newPassword))
	require.Equal(t, []revokedSessions{{userID: "u-ana", keepHash: "keep"}}, f.store.changed)
	require.Equal(t, "user.password_changed", f.publisher.events[0].Action)
}

func Test_AuditEventsCarryNoSecrets(t *testing.T) {
	f := newPeopleFixture(passwordConfig())
	withPeople(t, f)
	tokens := performAuditedActions(t, f)

	require.Equal(t, []string{
		"invite.created", "invite.created", "invite.revoked", "invite.accepted",
		"user.password_reset_issued", "user.password_reset", "user.password_changed", "user.removed",
	}, auditedActions(f))
	dump, err := json.Marshal(f.publisher.events)
	require.NoError(t, err)
	for _, secret := range secretsOf(f, tokens) {
		require.NotContains(t, string(dump), secret)
	}
}

func performAuditedActions(t *testing.T, f *peopleFixture) []string {
	t.Helper()
	kept := invite(t, f, maintainer, "places", "member")
	dropped := invite(t, f, maintainer, "places", "member")
	require.NoError(t, f.invites.Revoke(ctx, maintainer, "places", dropped.Link.ID))
	_, err := f.invites.Accept(ctx, tokenOf(t, kept.URL), "bea", password, password, false)
	require.NoError(t, err)
	reset, err := f.people.IssueReset(ctx, admin, "ana")
	require.NoError(t, err)
	require.NoError(t, f.people.UseReset(ctx, tokenOf(t, reset), newPassword, newPassword, ip))
	ana := actor.Actor{UserID: "u-ana", Login: "ana"}
	require.NoError(t, f.people.ChangePassword(ctx, ana, password, password+"!", password+"!", "keep"))
	require.NoError(t, f.people.Remove(ctx, admin, "bea"))
	return []string{tokenOf(t, kept.URL), tokenOf(t, dropped.URL), tokenOf(t, reset)}
}

func auditedActions(f *peopleFixture) []string {
	actions := []string{}
	for _, event := range f.publisher.events {
		actions = append(actions, event.Action)
	}
	return actions
}

func secretsOf(f *peopleFixture, tokens []string) []string {
	secrets := append([]string{password, newPassword, f.store.resets["u-ana"], f.users["ana"].PasswordHash, f.users["bea"].PasswordHash}, tokens...)
	for _, link := range f.links.all() {
		secrets = append(secrets, link.TokenHash)
	}
	return secrets
}
