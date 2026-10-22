package sessions

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
)

type SessionStore interface {
	Find(ctx context.Context, tokenHash string) (entities.Session, error)
	Extend(ctx context.Context, tokenHash string, expiresAt time.Time) error
	Delete(ctx context.Context, tokenHash string) error
}

type UserReader interface {
	FindByID(ctx context.Context, id string) (entities.User, error)
}

type TeamScope interface {
	VisibleTeamIDs(ctx context.Context, userID string) ([]string, error)
}

var errUnauthenticated = apperror.New(apperror.CodeUnauthenticated, "sign in first")

type SessionsUsecase struct {
	sessions    SessionStore
	users       UserReader
	teams       TeamScope
	ttl         time.Duration
	renewWithin time.Duration
}

func NewSessionsUsecase(cfg config.Config, sessions SessionStore, users UserReader, teams TeamScope) SessionsUsecase {
	return SessionsUsecase{sessions: sessions, users: users, teams: teams, ttl: cfg.Session.TTL, renewWithin: cfg.Session.RenewWithin}
}

func (u SessionsUsecase) Authenticate(ctx context.Context, token string) (entities.Authenticated, error) {
	if token == "" {
		return entities.Authenticated{}, errUnauthenticated
	}
	session, err := u.sessions.Find(ctx, entities.HashToken(token))
	if err != nil {
		return entities.Authenticated{}, err
	}
	now := clock.Now().UTC()
	if !now.Before(session.ExpiresAt) {
		return entities.Authenticated{}, errUnauthenticated
	}
	if err := u.renewIfDue(ctx, session, now); err != nil {
		return entities.Authenticated{}, err
	}
	return u.authenticated(ctx, session)
}

func (u SessionsUsecase) authenticated(ctx context.Context, session entities.Session) (entities.Authenticated, error) {
	user, err := u.users.FindByID(ctx, session.UserID)
	if err != nil {
		return entities.Authenticated{}, errUnauthenticated
	}
	teams, err := u.teams.VisibleTeamIDs(ctx, user.ID)
	return entities.Authenticated{User: user, Teams: teams, CSRFToken: session.CSRFToken}, err
}

func (u SessionsUsecase) renewIfDue(ctx context.Context, session entities.Session, now time.Time) error {
	if session.ExpiresAt.Sub(now) > u.renewWithin {
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
