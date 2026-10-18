package signin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/usecases/signin"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

type fakeGitHub struct {
	user entities.GitHubUser
	err  error
}

func (g fakeGitHub) Exchange(_ context.Context, code, _ string) (string, error) {
	return "token-for-" + code, g.err
}

func (g fakeGitHub) CurrentUser(context.Context, string) (entities.GitHubUser, error) {
	return g.user, g.err
}

type fakeUsers map[int64]entities.User

func (f fakeUsers) FindByGitHubID(_ context.Context, id int64) (entities.User, error) {
	user, ok := f[id]
	if !ok {
		return entities.User{}, apperror.NotFound("user not found")
	}
	return user, nil
}

func (f fakeUsers) Save(_ context.Context, user entities.User) error {
	f[user.GitHubID] = user
	return nil
}

type fakeSessions struct{ created []entities.Session }

func (f *fakeSessions) Create(_ context.Context, s entities.Session) error {
	f.created = append(f.created, s)
	return nil
}

func newUsecase(github fakeGitHub, users fakeUsers, sessions *fakeSessions) signin.SignInUsecase {
	clock.WithTime(now)
	cfg := config.Config{Admins: []string{"AdrianoZP"}, Session: config.Session{TTL: time.Hour}}
	return signin.NewSignInUsecase(cfg, github, users, sessions)
}

func Test_SignIn_CreatesTheUserAndASession(t *testing.T) {
	users, sessions := fakeUsers{}, &fakeSessions{}
	github := fakeGitHub{user: entities.GitHubUser{ID: 42, Login: "ana", Name: "Ana", AvatarURL: "a.png"}}

	issued, err := newUsecase(github, users, sessions).SignIn(context.Background(), "code", "http://x/cb")

	require.NoError(t, err)
	require.Equal(t, "ana", users[42].Login)
	require.False(t, users[42].Admin)
	require.NotEmpty(t, users[42].ID)
	require.Len(t, sessions.created, 1)
	require.Equal(t, entities.HashToken(issued.Token), sessions.created[0].TokenHash)
	require.NotEqual(t, issued.Token, sessions.created[0].TokenHash)
	require.Equal(t, now.Add(time.Hour), issued.ExpiresAt)
}

func Test_SignIn_UpdatesAnExistingUserAndKeepsItsID(t *testing.T) {
	users := fakeUsers{42: {ID: "u1", GitHubID: 42, Login: "old", Admin: true}}
	github := fakeGitHub{user: entities.GitHubUser{ID: 42, Login: "new"}}

	_, err := newUsecase(github, users, &fakeSessions{}).SignIn(context.Background(), "code", "")

	require.NoError(t, err)
	require.Equal(t, "u1", users[42].ID)
	require.Equal(t, "new", users[42].Login)
	require.True(t, users[42].Admin)
	require.Equal(t, now, users[42].LastLoginAt)
}

func Test_SignIn_ConfiguredAdminsAreAdminIgnoringCase(t *testing.T) {
	users := fakeUsers{}
	github := fakeGitHub{user: entities.GitHubUser{ID: 7, Login: "adrianozp"}}

	_, err := newUsecase(github, users, &fakeSessions{}).SignIn(context.Background(), "code", "")

	require.NoError(t, err)
	require.True(t, users[7].Admin)
}

func Test_SignIn_FailsWithoutCodeOrWhenGitHubFails(t *testing.T) {
	failing := fakeGitHub{err: errors.New("bad_verification_code")}

	_, noCode := newUsecase(fakeGitHub{}, fakeUsers{}, &fakeSessions{}).SignIn(context.Background(), "", "")
	_, githubDown := newUsecase(failing, fakeUsers{}, &fakeSessions{}).SignIn(context.Background(), "code", "")

	require.ErrorIs(t, noCode, apperror.New(apperror.CodeSignInFailed, ""))
	require.ErrorIs(t, githubDown, apperror.New(apperror.CodeSignInFailed, ""))
}
