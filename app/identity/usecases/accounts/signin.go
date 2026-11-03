package accounts

import (
	"context"
	"errors"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/internal/ratelimit"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/config"
)

const (
	failuresPerHour   = 10
	maxLoggedUsername = 64
)

var (
	errWrongCredentials = apperror.New(apperror.CodeSignInFailed, "Wrong username or password.")
	ErrThrottled        = apperror.New(apperror.CodeSignInFailed, "Too many attempts, try again later.")
)

type PasswordSignIn struct {
	sessionTTL time.Duration
	mode       Mode
	users      UserStore
	sessions   SessionStore
	logger     Logger
	failures   *ratelimit.Limiter
	verify     func(hash, password string) bool
}

func NewPasswordSignIn(cfg config.Config, mode Mode, users UserStore, sessions SessionStore, logger Logger) PasswordSignIn {
	return PasswordSignIn{
		sessionTTL: cfg.Session.TTL, mode: mode, users: users, sessions: sessions, logger: logger,
		failures: ratelimit.PerHour(failuresPerHour), verify: entities.VerifyPassword,
	}
}

func (p PasswordSignIn) WithVerifier(verify func(hash, password string) bool) PasswordSignIn {
	p.verify = verify
	return p
}

func (p PasswordSignIn) SignIn(ctx context.Context, username, password, ip string) (entities.IssuedSession, error) {
	if err := p.mode.RequirePasswords(); err != nil {
		return entities.IssuedSession{}, err
	}
	login, normErr := entities.NormalizeLogin(username)
	keys := []string{ipKey(ip), userKey(login, ip)}
	if !p.reserve(keys) {
		return entities.IssuedSession{}, ErrThrottled
	}
	user, ok, err := p.authenticate(ctx, login, normErr == nil, password)
	if err != nil || ok {
		p.refund(keys)
	}
	if err != nil {
		return entities.IssuedSession{}, err
	}
	if !ok {
		return entities.IssuedSession{}, p.fail(username, ip)
	}
	return p.succeed(ctx, user, ip)
}

func (p PasswordSignIn) succeed(ctx context.Context, user entities.User, ip string) (entities.IssuedSession, error) {
	p.failures.Clear(userKey(user.Login, ip))
	return startSession(ctx, p.sessions, user.ID, p.sessionTTL)
}

func (p PasswordSignIn) ClearFailures(login, ip string) {
	p.failures.Clear("user:" + login)
	p.failures.Clear(ipKey(ip))
}

func (p PasswordSignIn) reserve(keys []string) bool {
	for i, key := range keys {
		if !p.failures.Take(key) {
			p.refund(keys[:i])
			return false
		}
	}
	return true
}

func (p PasswordSignIn) refund(keys []string) {
	for _, key := range keys {
		p.failures.Refund(key)
	}
}

func (p PasswordSignIn) authenticate(ctx context.Context, login string, valid bool, password string) (entities.User, bool, error) {
	user, err := p.find(ctx, login, valid)
	if err != nil && !errors.Is(err, apperror.NotFound("")) {
		p.verify(entities.DummyHash, password)
		return entities.User{}, false, err
	}
	if err != nil || !user.Local() || user.Disabled() {
		p.verify(entities.DummyHash, password)
		return entities.User{}, false, nil
	}
	return user, p.verify(user.PasswordHash, password), nil
}

func (p PasswordSignIn) find(ctx context.Context, login string, valid bool) (entities.User, error) {
	if !valid {
		return entities.User{}, apperror.NotFound("user not found")
	}
	return p.users.FindByLogin(ctx, login)
}

func (p PasswordSignIn) fail(username, ip string) error {
	p.logger.Printf("INFO sign-in failed username=%q ip=%s", truncate(username), ip)
	return errWrongCredentials
}

func truncate(value string) string {
	runes := []rune(value)
	if len(runes) > maxLoggedUsername {
		return string(runes[:maxLoggedUsername])
	}
	return value
}

func ipKey(ip string) string {
	return "ip:" + ip
}

func userKey(login, ip string) string {
	return "user:" + login + "|" + ip
}
