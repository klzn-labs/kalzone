package accounts_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/usecases/accounts"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/stretchr/testify/require"
)

const ip = "10.0.0.1"

type brokenUsers struct{}

func (brokenUsers) FindByLogin(context.Context, string) (entities.User, error) {
	return entities.User{}, errors.New("database is down")
}

type verifier struct {
	mu     sync.Mutex
	hashes []string
}

func (v *verifier) verify(hash, password string) bool {
	v.mu.Lock()
	v.hashes = append(v.hashes, hash)
	v.mu.Unlock()
	return entities.VerifyPassword(hash, password)
}

func signInFixture(t *testing.T, cfg config.Config) (accounts.PasswordSignIn, *sessions, *logger) {
	t.Helper()
	clock.WithTime(now)
	cfg.Session.TTL = time.Hour
	hash, err := entities.HashPassword(password)
	require.NoError(t, err)
	disabledAt := now
	u := users{
		"ana":  {ID: "u-1", Login: "ana", PasswordHash: hash},
		"gone": {ID: "u-2", Login: "gone", PasswordHash: hash, DisabledAt: &disabledAt},
		"octo": {ID: "u-3", Login: "octo"},
	}
	s, l := &sessions{}, &logger{}
	return accounts.NewPasswordSignIn(cfg, accounts.NewMode(cfg), u, s, l), s, l
}

func withVerifier(uc accounts.PasswordSignIn) (accounts.PasswordSignIn, *verifier) {
	v := &verifier{}
	return uc.WithVerifier(v.verify), v
}

func Test_SignInWithThePassword(t *testing.T) {
	uc, sessions, _ := signInFixture(t, passwordConfig())

	issued, err := uc.SignIn(ctx, "ana", password, ip)

	require.NoError(t, err)
	require.Len(t, sessions.created, 1)
	require.Equal(t, "u-1", sessions.created[0].UserID)
	require.Equal(t, entities.HashToken(issued.Token), sessions.created[0].TokenHash)
	require.Equal(t, now.Add(time.Hour), issued.ExpiresAt)
}

func Test_SignInNormalizesTheUsername(t *testing.T) {
	uc, sessions, _ := signInFixture(t, passwordConfig())

	_, err := uc.SignIn(ctx, "  Ana ", password, ip)

	require.NoError(t, err)
	require.Len(t, sessions.created, 1)
}

func Test_UnknownWrongAndDisabledLookTheSame(t *testing.T) {
	uc, sessions, logs := signInFixture(t, passwordConfig())

	_, unknown := uc.SignIn(ctx, "nobody", password, ip)
	_, wrong := uc.SignIn(ctx, "ana", "not the password at all", ip)
	_, disabled := uc.SignIn(ctx, "gone", password, ip)
	_, noPassword := uc.SignIn(ctx, "octo", password, ip)
	_, malformed := uc.SignIn(ctx, "Bad Name!", password, ip)

	for _, err := range []error{wrong, disabled, noPassword, malformed} {
		require.Same(t, unknown, err)
	}
	require.ErrorIs(t, unknown, apperror.New(apperror.CodeSignInFailed, ""))
	require.Equal(t, "Wrong username or password.", unknown.Error())
	require.Empty(t, sessions.created)
	require.Equal(t, "INFO sign-in failed username=\"nobody\" ip=10.0.0.1", logs.lines[0])
	require.Len(t, logs.lines, 5)
}

func Test_EleventhFailureIsThrottledWithoutChecking(t *testing.T) {
	uc, sessions, logs := signInFixture(t, passwordConfig())
	for range 10 {
		_, err := uc.SignIn(ctx, "ana", "not the password at all", ip)
		require.Equal(t, "Wrong username or password.", err.Error())
	}

	_, err := uc.SignIn(ctx, "ana", password, ip)

	require.Equal(t, "Too many attempts, try again later.", err.Error())
	require.ErrorIs(t, err, apperror.New(apperror.CodeSignInFailed, ""))
	require.Empty(t, sessions.created)
	require.Len(t, logs.lines, 10)
}

func Test_OtherIPStillSignsIn(t *testing.T) {
	uc, _, _ := signInFixture(t, passwordConfig())
	for range 10 {
		_, _ = uc.SignIn(ctx, "ana", "not the password at all", ip)
	}

	_, sameIP := uc.SignIn(ctx, "ana", password, ip)
	_, otherIP := uc.SignIn(ctx, "ana", password, "10.0.0.2")

	require.Error(t, sameIP)
	require.NoError(t, otherIP)
}

func Test_ClearFailuresLiftsTheBlockOnTheSameIP(t *testing.T) {
	uc, _, _ := signInFixture(t, passwordConfig())
	for range 10 {
		_, _ = uc.SignIn(ctx, "ana", "not the password at all", ip)
	}
	_, blocked := uc.SignIn(ctx, "ana", password, ip)

	uc.ClearFailures("ana", ip)

	_, err := uc.SignIn(ctx, "ana", password, ip)
	require.Equal(t, "Too many attempts, try again later.", blocked.Error())
	require.NoError(t, err)
}

func Test_SuccessClearsTheUsernameAndIPFailures(t *testing.T) {
	uc, _, _ := signInFixture(t, passwordConfig())
	failTimes(uc, "ana", 9)
	_, err := uc.SignIn(ctx, "ana", password, ip)
	require.NoError(t, err)
	uc.ClearFailures("bea", ip)

	for range 10 {
		_, err := uc.SignIn(ctx, "ana", "not the password at all", ip)
		require.Equal(t, "Wrong username or password.", err.Error())
	}
}

func failTimes(uc accounts.PasswordSignIn, login string, n int) {
	for range n {
		_, _ = uc.SignIn(ctx, login, "not the password at all", ip)
	}
}

func Test_ConcurrentWrongAttemptsNeverExceedTheLimit(t *testing.T) {
	uc, _, _ := signInFixture(t, passwordConfig())
	uc, v := withVerifier(uc)
	var throttled atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := uc.SignIn(ctx, "ana", "not the password at all", ip)
			if err.Error() == "Too many attempts, try again later." {
				throttled.Add(1)
			}
		}()
	}
	wg.Wait()

	require.Len(t, v.hashes, 10)
	require.EqualValues(t, 10, throttled.Load())
}

func Test_DummyHashIsVerifiedForUnknownDisabledAndMalformed(t *testing.T) {
	uc, _, _ := signInFixture(t, passwordConfig())
	uc, v := withVerifier(uc)

	_, _ = uc.SignIn(ctx, "nobody", password, ip)
	_, _ = uc.SignIn(ctx, "gone", password, ip)
	_, _ = uc.SignIn(ctx, "Bad Name!", password, ip)

	require.Equal(t, []string{entities.DummyHash, entities.DummyHash, entities.DummyHash}, v.hashes)
}

func Test_StoreErrorsAreReturnedAndNotCounted(t *testing.T) {
	cfg := passwordConfig()
	s, l := &sessions{}, &logger{}
	uc := accounts.NewPasswordSignIn(cfg, accounts.NewMode(cfg), brokenUsers{}, s, l)
	uc, v := withVerifier(uc)

	for range 12 {
		_, err := uc.SignIn(ctx, "ana", password, ip)
		require.EqualError(t, err, "database is down")
	}

	require.Len(t, v.hashes, 12)
	require.Empty(t, l.lines)
}

func Test_FailedSignInLogQuotesTheUsername(t *testing.T) {
	uc, _, logs := signInFixture(t, passwordConfig())

	_, _ = uc.SignIn(ctx, "x\nINFO forged", password, ip)

	require.Equal(t, []string{`INFO sign-in failed username="x\nINFO forged" ip=10.0.0.1`}, logs.lines)
}

func Test_GitHubModeRefusesPasswordSignIn(t *testing.T) {
	uc, sessions, _ := signInFixture(t, githubConfig())

	_, err := uc.SignIn(ctx, "ana", password, ip)

	require.ErrorIs(t, err, apperror.New(apperror.CodeGitHubMode, ""))
	require.Empty(t, sessions.created)
}
