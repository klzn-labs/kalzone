package sessions

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/demo"
)

type SessionStore interface {
	Create(ctx context.Context, session entities.Session) error
	Find(ctx context.Context, tokenHash string) (entities.Session, error)
	Extend(ctx context.Context, tokenHash string, expiresAt time.Time) error
	Delete(ctx context.Context, tokenHash string) error
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
}

type UserReader interface {
	FindByID(ctx context.Context, id string) (entities.User, error)
}

type TeamScope interface {
	VisibleTeamIDs(ctx context.Context, userID string) ([]string, error)
}

var (
	errUnauthenticated = apperror.New(apperror.CodeUnauthenticated, "sign in first")
	errNoDemo          = apperror.NotFound("the demo is not open on this platform").WithHint("sign in with GitHub instead")
)

type SessionsUsecase struct {
	sessions    SessionStore
	users       UserReader
	teams       TeamScope
	ttl         time.Duration
	renewWithin time.Duration
	demoTTL     time.Duration
	demo        bool
}

func NewSessionsUsecase(cfg config.Config, sessions SessionStore, users UserReader, teams TeamScope) SessionsUsecase {
	return SessionsUsecase{
		sessions: sessions, users: users, teams: teams, ttl: cfg.Session.TTL, renewWithin: cfg.Session.RenewWithin,
		demoTTL: cfg.Demo.SessionTTL, demo: cfg.Demo.Enabled,
	}
}

func (u SessionsUsecase) Authenticate(ctx context.Context, token string) (entities.Authenticated, error) {
	now := clock.Now().UTC()
	session, err := u.live(ctx, token, now)
	if err != nil {
		return entities.Authenticated{}, err
	}
	user, err := u.users.FindByID(ctx, session.UserID)
	if err != nil {
		return entities.Authenticated{}, errUnauthenticated
	}
	if err := u.renewIfDue(ctx, session, user, now); err != nil {
		return entities.Authenticated{}, err
	}
	teams, err := u.teams.VisibleTeamIDs(ctx, user.ID)
	return entities.Authenticated{User: user, Teams: teams, CSRFToken: session.CSRFToken}, err
}

func (u SessionsUsecase) live(ctx context.Context, token string, now time.Time) (entities.Session, error) {
	if token == "" {
		return entities.Session{}, errUnauthenticated
	}
	session, err := u.sessions.Find(ctx, entities.HashToken(token))
	if err != nil {
		return entities.Session{}, err
	}
	if !now.Before(session.ExpiresAt) {
		return entities.Session{}, errUnauthenticated
	}
	return session, nil
}

func (u SessionsUsecase) renewIfDue(ctx context.Context, session entities.Session, user entities.User, now time.Time) error {
	if user.Guest() || session.ExpiresAt.Sub(now) > u.renewWithin {
		return nil
	}
	return u.sessions.Extend(ctx, session.TokenHash, now.Add(u.ttl))
}

func (u SessionsUsecase) SignOut(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return u.sessions.Delete(ctx, entities.HashToken(token))
}

func (u SessionsUsecase) EnterDemo(ctx context.Context) (entities.IssuedSession, error) {
	if !u.demo {
		return entities.IssuedSession{}, errNoDemo
	}
	if _, err := u.users.FindByID(ctx, demo.GuestID); err != nil {
		return entities.IssuedSession{}, errNoDemo
	}
	session, issued, err := entities.OpenSession(demo.GuestID, clock.Now().UTC(), u.demoTTL)
	if err != nil {
		return entities.IssuedSession{}, err
	}
	return issued, u.sessions.Create(ctx, session)
}

func (u SessionsUsecase) Sweep(ctx context.Context) (int, error) {
	return u.sessions.DeleteExpired(ctx, clock.Now().UTC())
}
