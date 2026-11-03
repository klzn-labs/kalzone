package signin

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

type GitHub interface {
	Exchange(ctx context.Context, code, redirectURI string) (string, error)
	CurrentUser(ctx context.Context, accessToken string) (entities.GitHubUser, error)
}

type UserStore interface {
	FindByGitHubID(ctx context.Context, githubID int64) (entities.User, error)
	Save(ctx context.Context, user entities.User) error
}

type SessionStore interface {
	Create(ctx context.Context, session entities.Session) error
}

var errSignInFailed = apperror.New(apperror.CodeSignInFailed, "GitHub sign-in failed, try again")

type SignInUsecase struct {
	github   GitHub
	users    UserStore
	sessions SessionStore
	admins   map[string]bool
	ttl      time.Duration
}

func NewSignInUsecase(cfg config.Config, github GitHub, users UserStore, sessions SessionStore) SignInUsecase {
	admins := map[string]bool{}
	for _, login := range cfg.Admins {
		admins[strings.ToLower(strings.TrimSpace(login))] = true
	}
	return SignInUsecase{github: github, users: users, sessions: sessions, admins: admins, ttl: cfg.Session.TTL}
}

func (u SignInUsecase) SignIn(ctx context.Context, code, redirectURI string) (entities.IssuedSession, error) {
	githubUser, err := u.fetchGitHubUser(ctx, code, redirectURI)
	if err != nil {
		return entities.IssuedSession{}, err
	}
	user, err := u.upsert(ctx, githubUser)
	if err != nil {
		return entities.IssuedSession{}, err
	}
	return u.issue(ctx, user.ID)
}

func (u SignInUsecase) fetchGitHubUser(ctx context.Context, code, redirectURI string) (entities.GitHubUser, error) {
	if code == "" {
		return entities.GitHubUser{}, errSignInFailed
	}
	token, err := u.github.Exchange(ctx, code, redirectURI)
	if err != nil {
		return entities.GitHubUser{}, errors.Join(errSignInFailed, err)
	}
	user, err := u.github.CurrentUser(ctx, token)
	if err != nil {
		return entities.GitHubUser{}, errors.Join(errSignInFailed, err)
	}
	return user, nil
}

func (u SignInUsecase) upsert(ctx context.Context, g entities.GitHubUser) (entities.User, error) {
	now := clock.Now().UTC()
	user, err := u.users.FindByGitHubID(ctx, g.ID)
	if errors.Is(err, apperror.NotFound("")) {
		user, err = entities.User{ID: uuid.NewUUID(), GitHubID: g.ID, CreatedAt: now}, nil
	}
	if err != nil {
		return entities.User{}, err
	}
	if user.Disabled() {
		return entities.User{}, errSignInFailed
	}
	user.Login, user.Name, user.AvatarURL, user.LastLoginAt = g.Login, g.Name, g.AvatarURL, now
	user.Admin = user.Admin || u.admins[strings.ToLower(g.Login)]
	return user, u.users.Save(ctx, user)
}

func (u SignInUsecase) issue(ctx context.Context, userID string) (entities.IssuedSession, error) {
	session, issued, err := entities.OpenSession(userID, clock.Now().UTC(), u.ttl)
	if err != nil {
		return entities.IssuedSession{}, err
	}
	return issued, u.sessions.Create(ctx, session)
}
