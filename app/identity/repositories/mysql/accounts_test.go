package mysql_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newLink(id string, kind entities.LinkKind, expires time.Time) entities.AccountLink {
	return entities.AccountLink{ID: id, Kind: kind, TokenHash: id + "-hash", TeamID: "t1", Role: "viewer", CreatedAt: created, ExpiresAt: expires}
}

func count(t *testing.T, gdb *gorm.DB, table, where string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, gdb.Table(table).Where(where, args...).Count(&n).Error)
	return n
}

func Test_SetupClaimsTheLinkOnce(t *testing.T) {
	ctx := context.Background()
	gdb := testdb.Open(t)
	require.NoError(t, mysql.NewAccountLinkRepository(gdb).Create(ctx, newLink("l1", entities.LinkSetup, time.Now().Add(time.Hour))))
	repo := mysql.NewAccountRepository(gdb)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = completeSetup(ctx, repo, strconv.Itoa(i))
		}()
	}
	wg.Wait()

	require.Equal(t, 1, countNil(errs))
	require.EqualError(t, firstError(errs), "This link is no longer valid.")
	require.EqualValues(t, 1, count(t, gdb, "users", "1 = 1"))
	require.EqualValues(t, 1, count(t, gdb, "memberships", "role = ?", "maintainer"))
}

func completeSetup(ctx context.Context, repo mysql.AccountRepository, n string) error {
	owner := entities.User{ID: "u" + n, Login: "owner" + n, CreatedAt: created, LastLoginAt: created}
	return repo.CompleteSetup(ctx, "l1", owner, entities.NewTeam{ID: "t" + n, Slug: "default" + n, Name: "Default", CreatedAt: created})
}

func firstError(errs []error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func countNil(errs []error) int {
	n := 0
	for _, err := range errs {
		if err == nil {
			n++
		}
	}
	return n
}

func Test_DisableEndsAccess(t *testing.T) {
	ctx := context.Background()
	gdb := testdb.Open(t)
	seedRemovable(t, gdb)

	require.NoError(t, mysql.NewAccountRepository(gdb).Disable(ctx, "u1", created))

	require.Zero(t, count(t, gdb, "memberships", "user_id = ?", "u1"))
	require.Zero(t, count(t, gdb, "sessions", "user_id = ?", "u1"))
	require.EqualValues(t, 1, count(t, gdb, "access_tokens", "id = ? AND revoked_at IS NOT NULL", "k1"))
	require.EqualValues(t, 2, count(t, gdb, "account_links", "id IN ('r1', 'i1') AND revoked_at IS NOT NULL"))
	require.EqualValues(t, 1, count(t, gdb, "account_links", "id = 'i2' AND revoked_at IS NULL"))
	require.EqualValues(t, 1, count(t, gdb, "users", "id = ? AND disabled_at IS NOT NULL AND is_admin = FALSE", "u1"))
}

func seedRemovable(t *testing.T, gdb *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	saveUser(t, gdb, entities.User{ID: "u1", Login: "ana", Admin: true})
	saveUser(t, gdb, entities.User{ID: "u2", Login: "bea", Admin: true})
	require.NoError(t, gdb.Exec("INSERT INTO memberships (team_id, user_id, role, created_at) VALUES ('t1', 'u1', 'viewer', ?)", created).Error)
	require.NoError(t, mysql.NewSessionRepository(gdb).Create(ctx, entities.Session{TokenHash: "s1", UserID: "u1", CreatedAt: created, ExpiresAt: created.Add(time.Hour)}))
	token := entities.AccessToken{ID: "k1", OwnerID: "u1", Name: "ci", Hash: "h1", CreatedAt: created, ExpiresAt: created.Add(time.Hour)}
	require.NoError(t, mysql.NewTokenRepository(gdb).Create(ctx, token))
	reset, mine, theirs := newLink("r1", entities.LinkReset, time.Now().Add(time.Hour)), newLink("i1", entities.LinkInvite, time.Now().Add(time.Hour)), newLink("i2", entities.LinkInvite, time.Now().Add(time.Hour))
	reset.UserID, mine.CreatedBy, theirs.CreatedBy = "u1", "u1", "u2"
	for _, l := range []entities.AccountLink{reset, mine, theirs} {
		require.NoError(t, mysql.NewAccountLinkRepository(gdb).Create(ctx, l))
	}
}

func saveUser(t *testing.T, gdb *gorm.DB, u entities.User) {
	t.Helper()
	u.CreatedAt, u.LastLoginAt = created, created
	require.NoError(t, mysql.NewUserRepository(gdb).Save(context.Background(), u))
}

func Test_DisableKeepsTheLastAdmin(t *testing.T) {
	ctx := context.Background()
	gdb := testdb.Open(t)
	saveUser(t, gdb, entities.User{ID: "u1", Login: "ana", Admin: true})
	saveUser(t, gdb, entities.User{ID: "u2", Login: "bea"})

	err := mysql.NewAccountRepository(gdb).Disable(ctx, "u1", created)

	require.ErrorIs(t, err, apperror.Conflict(""))
	require.EqualError(t, err, "keep at least one admin")
	require.Zero(t, count(t, gdb, "users", "disabled_at IS NOT NULL"))
	require.NoError(t, mysql.NewAccountRepository(gdb).Disable(ctx, "u2", created))
}

func Test_TwoAdminsRemovingEachOtherLeaveOne(t *testing.T) {
	gdb := testdb.Open(t)
	saveUser(t, gdb, entities.User{ID: "u1", Login: "ana", Admin: true})
	saveUser(t, gdb, entities.User{ID: "u2", Login: "bea", Admin: true})
	repo := mysql.NewAccountRepository(gdb)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, id := range []string{"u1", "u2"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = repo.Disable(context.Background(), id, created)
		}()
	}
	wg.Wait()

	require.Equal(t, 1, countNil(errs))
	require.EqualError(t, firstError(errs), "keep at least one admin")
	require.EqualValues(t, 1, count(t, gdb, "users", "is_admin AND disabled_at IS NULL"))
}

func Test_DisableOfAMemberAndAnAcceptedInviteBothSucceed(t *testing.T) {
	gdb := testdb.Open(t)
	require.NoError(t, gdb.Exec("INSERT INTO teams (id, slug, name, created_at) VALUES ('t1', 'places', 'Places', ?)", created).Error)
	saveUser(t, gdb, entities.User{ID: "admin", Login: "root", Admin: true})
	for i := range 5 {
		n := strconv.Itoa(i)
		saveUser(t, gdb, entities.User{ID: "member" + n, Login: "member" + n})
		link := newLink("invite"+n, entities.LinkInvite, time.Now().Add(time.Hour))
		link.CreatedBy = "admin"
		require.NoError(t, mysql.NewAccountLinkRepository(gdb).Create(context.Background(), link))
		require.Equal(t, []error{nil, nil}, disableWhileAccepting(gdb, link, n))
	}
	require.EqualValues(t, 5, count(t, gdb, "users", "disabled_at IS NOT NULL"))
	require.EqualValues(t, 5, count(t, gdb, "memberships", "team_id = 't1'"))
}

func disableWhileAccepting(gdb *gorm.DB, link entities.AccountLink, n string) []error {
	repo := mysql.NewAccountRepository(gdb)
	invited := entities.User{ID: "invited" + n, Login: "invited" + n, CreatedAt: created, LastLoginAt: created}
	return together(
		func() error { return repo.Disable(context.Background(), "member"+n, created) },
		func() error { return repo.AcceptInvite(context.Background(), link, invited) },
	)
}

func together(calls ...func() error) []error {
	errs := make([]error, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = call()
		}()
	}
	wg.Wait()
	return errs
}

func Test_DisableRacingTheirOwnResetEndsDisabled(t *testing.T) {
	gdb := testdb.Open(t)
	saveUser(t, gdb, entities.User{ID: "admin", Login: "root", Admin: true})
	repo := mysql.NewAccountRepository(gdb)
	for i := range 5 {
		n := strconv.Itoa(i)
		saveUser(t, gdb, entities.User{ID: "member" + n, Login: "member" + n})
		link := newLink("reset"+n, entities.LinkReset, time.Now().Add(time.Hour))
		link.UserID = "member" + n
		require.NoError(t, mysql.NewAccountLinkRepository(gdb).Create(context.Background(), link))
		errs := together(
			func() error { return repo.Disable(context.Background(), "member"+n, created) },
			func() error { return repo.ResetPassword(context.Background(), link, "newhash") },
		)
		require.NoError(t, errs[0])
		requireNilOrLinkGone(t, errs[1])
		requireLinkSettled(t, gdb, link.ID)
	}
	require.EqualValues(t, 5, count(t, gdb, "users", "disabled_at IS NOT NULL"))
}

func Test_DisableOfAMaintainerRacingTheirInviteSettlesBoth(t *testing.T) {
	gdb := testdb.Open(t)
	require.NoError(t, gdb.Exec("INSERT INTO teams (id, slug, name, created_at) VALUES ('t1', 'places', 'Places', ?)", created).Error)
	saveUser(t, gdb, entities.User{ID: "admin", Login: "root", Admin: true})
	for i := range 5 {
		n := strconv.Itoa(i)
		saveUser(t, gdb, entities.User{ID: "member" + n, Login: "member" + n})
		require.NoError(t, gdb.Exec("INSERT INTO memberships (team_id, user_id, role, created_at) VALUES ('t1', ?, 'maintainer', ?)", "member"+n, created).Error)
		link := newLink("invite"+n, entities.LinkInvite, time.Now().Add(time.Hour))
		link.CreatedBy = "member" + n
		require.NoError(t, mysql.NewAccountLinkRepository(gdb).Create(context.Background(), link))
		errs := disableWhileAccepting(gdb, link, n)
		require.NoError(t, errs[0])
		requireNilOrLinkGone(t, errs[1])
		requireInviteSettled(t, gdb, link.ID, "invited"+n, errs[1] == nil)
	}
	require.Zero(t, count(t, gdb, "memberships", "user_id LIKE 'member%'"))
}

func requireNilOrLinkGone(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		require.EqualError(t, err, "This link is no longer valid.")
	}
}

func requireLinkSettled(t *testing.T, gdb *gorm.DB, linkID string) {
	t.Helper()
	require.EqualValues(t, 1, count(t, gdb, "account_links", "id = ? AND (used_at IS NULL) <> (revoked_at IS NULL)", linkID))
}

func requireInviteSettled(t *testing.T, gdb *gorm.DB, linkID, invitedID string, accepted bool) {
	t.Helper()
	requireLinkSettled(t, gdb, linkID)
	want := int64(0)
	if accepted {
		want = 1
	}
	require.Equal(t, want, count(t, gdb, "users", "id = ?", invitedID))
	require.Equal(t, want, count(t, gdb, "memberships", "user_id = ? AND team_id = 't1'", invitedID))
}

func Test_ChangePasswordSetsHashAndKeepsOnlyThisSession(t *testing.T) {
	ctx := context.Background()
	gdb := testdb.Open(t)
	saveUser(t, gdb, entities.User{ID: "u1", Login: "ana"})
	saveUser(t, gdb, entities.User{ID: "u2", Login: "bea"})
	sessions := mysql.NewSessionRepository(gdb)
	for _, s := range []entities.Session{{TokenHash: "keep", UserID: "u1"}, {TokenHash: "drop", UserID: "u1"}, {TokenHash: "other", UserID: "u2"}} {
		s.CreatedAt, s.ExpiresAt = created, created.Add(time.Hour)
		require.NoError(t, sessions.Create(ctx, s))
	}

	require.NoError(t, mysql.NewAccountRepository(gdb).ChangePassword(ctx, "u1", "newhash", "keep"))

	got, err := mysql.NewUserRepository(gdb).FindByID(ctx, "u1")
	require.NoError(t, err)
	require.Equal(t, "newhash", got.PasswordHash)
	require.EqualValues(t, 2, count(t, gdb, "sessions", "token_hash IN ('keep', 'other')"))
	require.Zero(t, count(t, gdb, "sessions", "token_hash = 'drop'"))
}

func Test_SetupMapsALoginTakenByAnotherKindOfUser(t *testing.T) {
	ctx := context.Background()
	gdb := testdb.Open(t)
	saveUser(t, gdb, entities.User{ID: "u-guest", Login: "owner1", Kind: entities.KindGuest})
	require.NoError(t, mysql.NewAccountLinkRepository(gdb).Create(ctx, newLink("l1", entities.LinkSetup, time.Now().Add(time.Hour))))

	err := completeSetup(ctx, mysql.NewAccountRepository(gdb), "1")

	require.ErrorIs(t, err, apperror.Conflict(""))
	require.EqualError(t, err, "this username is taken")
	require.Zero(t, count(t, gdb, "account_links", "used_at IS NOT NULL"))
}

func Test_AcceptInviteRefusesADeletedTeam(t *testing.T) {
	ctx := context.Background()
	gdb := testdb.Open(t)
	link := newLink("l1", entities.LinkInvite, time.Now().Add(time.Hour))
	require.NoError(t, mysql.NewAccountLinkRepository(gdb).Create(ctx, link))
	repo := mysql.NewAccountRepository(gdb)

	err := repo.AcceptInvite(ctx, link, entities.User{ID: "u1", Login: "bob", CreatedAt: created, LastLoginAt: created})
	require.EqualError(t, err, "This link is no longer valid.")
	require.Zero(t, count(t, gdb, "users", "1 = 1"))

	require.NoError(t, gdb.Exec("INSERT INTO teams (id, slug, name, created_at) VALUES ('t1', 'places', 'Places', ?)", created).Error)
	require.NoError(t, repo.AcceptInvite(ctx, link, entities.User{ID: "u1", Login: "bob", CreatedAt: created, LastLoginAt: created}))
	require.EqualValues(t, 1, count(t, gdb, "memberships", "team_id = 't1' AND user_id = 'u1'"))
}

func Test_RevokeUnusedLeavesUsedLinks(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewAccountLinkRepository(testdb.Open(t))
	used := newLink("used", entities.LinkSetup, created.Add(time.Hour))
	used.UsedAt = &created
	require.NoError(t, repo.Create(ctx, used))
	require.NoError(t, repo.Create(ctx, newLink("open", entities.LinkSetup, created.Add(time.Hour))))

	require.NoError(t, repo.RevokeUnused(ctx, entities.LinkSetup, created))

	open, err := repo.FindByHash(ctx, "open-hash")
	require.NoError(t, err)
	kept, err := repo.FindByHash(ctx, "used-hash")
	require.NoError(t, err)
	require.NotNil(t, open.RevokedAt)
	require.Nil(t, kept.RevokedAt)
}

func Test_ListInvitesSkipsDeadLinks(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewAccountLinkRepository(testdb.Open(t))
	now := created.Add(time.Hour)
	used := newLink("used", entities.LinkInvite, now.Add(time.Hour))
	used.UsedAt = &now
	revoked := newLink("revoked", entities.LinkInvite, now.Add(time.Hour))
	revoked.RevokedAt = &now
	for _, l := range []entities.AccountLink{newLink("live", entities.LinkInvite, now.Add(time.Hour)), newLink("expired", entities.LinkInvite, now.Add(-time.Minute)), used, revoked} {
		require.NoError(t, repo.Create(ctx, l))
	}

	links, err := repo.ListInvites(ctx, "t1", now)

	require.NoError(t, err)
	require.Len(t, links, 1)
	require.Equal(t, "live", links[0].ID)
}

func Test_ListInvitesNamesWhoMadeThem(t *testing.T) {
	ctx := context.Background()
	gdb := testdb.Open(t)
	repo := mysql.NewAccountLinkRepository(gdb)
	require.NoError(t, mysql.NewUserRepository(gdb).Save(ctx, entities.User{ID: "u-maint", Login: "maint", CreatedAt: created, LastLoginAt: created}))
	link := newLink("live", entities.LinkInvite, created.Add(2*time.Hour))
	link.CreatedBy = "u-maint"
	require.NoError(t, repo.Create(ctx, link))

	links, err := repo.ListInvites(ctx, "t1", created.Add(time.Hour))

	require.NoError(t, err)
	require.Len(t, links, 1)
	require.Equal(t, "maint", links[0].CreatedByLogin)
	require.Equal(t, "u-maint", links[0].CreatedBy)
}

func Test_SaveKeepsDisabledAndPassword(t *testing.T) {
	ctx := context.Background()
	gdb := testdb.Open(t)
	users := mysql.NewUserRepository(gdb)
	require.NoError(t, users.Save(ctx, entities.User{ID: "u1", Login: "ana", CreatedAt: created, LastLoginAt: created}))
	require.NoError(t, users.SetPassword(ctx, "u1", "hash"))
	stale, err := users.FindByID(ctx, "u1")
	require.NoError(t, err)
	require.NoError(t, mysql.NewAccountRepository(gdb).Disable(ctx, "u1", created))

	stale.Admin = true
	require.NoError(t, users.Save(ctx, stale))

	got, err := users.FindByID(ctx, "u1")
	require.NoError(t, err)
	require.True(t, got.Disabled())
	require.Equal(t, "hash", got.PasswordHash)
}

func Test_AcceptInviteMapsTakenUsername(t *testing.T) {
	ctx := context.Background()
	gdb := testdb.Open(t)
	links := mysql.NewAccountLinkRepository(gdb)
	require.NoError(t, mysql.NewUserRepository(gdb).Save(ctx, entities.User{ID: "u1", Login: "ana", CreatedAt: created, LastLoginAt: created}))
	link := newLink("l1", entities.LinkInvite, time.Now().Add(time.Hour))
	require.NoError(t, links.Create(ctx, link))

	err := mysql.NewAccountRepository(gdb).AcceptInvite(ctx, link, entities.User{ID: "u2", Login: "ana", CreatedAt: created, LastLoginAt: created})

	require.EqualError(t, err, "this username is taken")
}

func Test_ClaimRefusesDeadLinks(t *testing.T) {
	now := time.Now()
	used, revoked := now, now
	cases := map[string]entities.AccountLink{
		"expired": newLink("l1", entities.LinkInvite, now.Add(-time.Minute)),
		"used":    withState(newLink("l1", entities.LinkInvite, now.Add(time.Hour)), &used, nil),
		"revoked": withState(newLink("l1", entities.LinkInvite, now.Add(time.Hour)), nil, &revoked),
	}
	for name, link := range cases {
		t.Run(name, func(t *testing.T) {
			gdb := testdb.Open(t)
			require.NoError(t, mysql.NewAccountLinkRepository(gdb).Create(context.Background(), link))

			err := mysql.NewAccountRepository(gdb).AcceptInvite(context.Background(), link, entities.User{ID: "u1", Login: "bob", CreatedAt: created, LastLoginAt: created})

			require.EqualError(t, err, "This link is no longer valid.")
			require.Zero(t, count(t, gdb, "users", "1 = 1"))
		})
	}
}

func withState(l entities.AccountLink, used, revoked *time.Time) entities.AccountLink {
	l.UsedAt, l.RevokedAt = used, revoked
	return l
}

func Test_ResetPasswordSetsHashAndDropsSessions(t *testing.T) {
	ctx := context.Background()
	gdb := testdb.Open(t)
	users := mysql.NewUserRepository(gdb)
	require.NoError(t, users.Save(ctx, entities.User{ID: "u1", Login: "ana", CreatedAt: created, LastLoginAt: created}))
	require.NoError(t, mysql.NewSessionRepository(gdb).Create(ctx, entities.Session{TokenHash: "s1", UserID: "u1", CreatedAt: created, ExpiresAt: created.Add(time.Hour)}))
	link := newLink("l1", entities.LinkReset, time.Now().Add(time.Hour))
	link.UserID = "u1"
	require.NoError(t, mysql.NewAccountLinkRepository(gdb).Create(ctx, link))

	require.NoError(t, mysql.NewAccountRepository(gdb).ResetPassword(ctx, link, "newhash"))

	got, err := users.FindByID(ctx, "u1")
	require.NoError(t, err)
	require.Equal(t, "newhash", got.PasswordHash)
	require.Zero(t, count(t, gdb, "sessions", "user_id = ?", "u1"))
}

func Test_SetupRefusesWhenAPersonExists(t *testing.T) {
	ctx := context.Background()
	gdb := testdb.Open(t)
	require.NoError(t, mysql.NewUserRepository(gdb).Save(ctx, entities.User{ID: "u-ana", Login: "ana", Kind: entities.KindPerson, CreatedAt: created, LastLoginAt: created}))
	require.NoError(t, mysql.NewAccountLinkRepository(gdb).Create(ctx, newLink("l1", entities.LinkSetup, time.Now().Add(time.Hour))))

	err := completeSetup(ctx, mysql.NewAccountRepository(gdb), "1")

	require.ErrorIs(t, err, apperror.Conflict(""))
	require.EqualError(t, err, "this Kalzone already has an owner")
	require.EqualValues(t, 1, count(t, gdb, "users", "1 = 1"))
	require.EqualValues(t, 0, count(t, gdb, "teams", "id = ?", "t1"))
	require.EqualValues(t, 0, count(t, gdb, "memberships", "user_id = ?", "u1"))
	require.EqualValues(t, 0, count(t, gdb, "account_links", "used_at IS NOT NULL"))
}

func Test_SetupRefusesWhenTheDefaultTeamExists(t *testing.T) {
	ctx := context.Background()
	gdb := testdb.Open(t)
	require.NoError(t, gdb.Exec("INSERT INTO teams (id, slug, name, created_at) VALUES ('t-old', 'default', 'Default', ?)", created).Error)
	require.NoError(t, mysql.NewAccountLinkRepository(gdb).Create(ctx, newLink("l1", entities.LinkSetup, time.Now().Add(time.Hour))))
	owner := entities.User{ID: "u1", Login: "owner", CreatedAt: created, LastLoginAt: created}
	team := entities.NewTeam{ID: "t1", Slug: "default", Name: "Default", CreatedAt: created}

	err := mysql.NewAccountRepository(gdb).CompleteSetup(ctx, "l1", owner, team)

	require.ErrorIs(t, err, apperror.Conflict(""))
	require.EqualError(t, err, "this Kalzone already has an owner")
	require.EqualValues(t, 0, count(t, gdb, "users", "1 = 1"))
	require.EqualValues(t, 0, count(t, gdb, "account_links", "used_at IS NOT NULL"))
}

func Test_RevokeUnusedForTouchesOnlyThatUsersLinksOfThatKind(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewAccountLinkRepository(testdb.Open(t))
	expires := created.Add(time.Hour)
	mine, other, invite := newLink("mine", entities.LinkReset, expires), newLink("other", entities.LinkReset, expires), newLink("invite", entities.LinkInvite, expires)
	mine.UserID, other.UserID, invite.UserID = "u1", "u2", "u1"
	for _, l := range []entities.AccountLink{mine, other, invite} {
		require.NoError(t, repo.Create(ctx, l))
	}

	require.NoError(t, repo.RevokeUnusedFor(ctx, entities.LinkReset, "u1", created))

	for hash, revoked := range map[string]bool{"mine-hash": true, "other-hash": false, "invite-hash": false} {
		link, err := repo.FindByHash(ctx, hash)
		require.NoError(t, err)
		require.Equal(t, revoked, link.RevokedAt != nil, hash)
	}
}
