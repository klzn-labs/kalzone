package accounts

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

type linkIssuer struct {
	baseURL string
	links   LinkStore
}

func (l linkIssuer) issue(ctx context.Context, draft entities.AccountLink) (entities.AccountLink, string, error) {
	link, token, err := entities.NewAccountLink(draft.Kind, clock.Now().UTC())
	if err != nil {
		return entities.AccountLink{}, "", err
	}
	link.TeamID, link.Role, link.UserID, link.CreatedBy = draft.TeamID, draft.Role, draft.UserID, draft.CreatedBy
	if err := l.links.Create(ctx, link); err != nil {
		return entities.AccountLink{}, "", err
	}
	return link, l.baseURL + "/join/" + token, nil
}

func (l linkIssuer) reset(ctx context.Context, user entities.User, createdBy string) (string, error) {
	if user.Disabled() || !user.Local() {
		return "", errNotResettable
	}
	if err := l.links.RevokeUnusedFor(ctx, entities.LinkReset, user.ID, clock.Now().UTC()); err != nil {
		return "", err
	}
	_, url, err := l.issue(ctx, entities.AccountLink{Kind: entities.LinkReset, UserID: user.ID, CreatedBy: createdBy})
	return url, err
}

func (l linkIssuer) live(ctx context.Context, token string, kind entities.LinkKind) (entities.AccountLink, error) {
	link, err := l.links.FindByHash(ctx, entities.HashToken(token))
	if err != nil {
		return entities.AccountLink{}, err
	}
	if link.Kind != kind || !link.Live(clock.Now().UTC()) {
		return entities.AccountLink{}, errLinkGone
	}
	return link, nil
}

func checkedPassword(password, confirm string) (string, error) {
	if err := entities.CheckPassword(password); err != nil {
		return "", err
	}
	if password != confirm {
		return "", errPasswordsDiffer
	}
	return entities.HashPassword(password)
}

func newPerson(in SetupInput) (entities.User, error) {
	login, hash, err := checkedCredentials(in)
	if err != nil {
		return entities.User{}, err
	}
	now := clock.Now().UTC()
	return entities.User{
		ID: uuid.NewUUID(), Kind: entities.KindPerson, Login: login, Name: login,
		CreatedAt: now, LastLoginAt: now, PasswordHash: hash,
	}, nil
}

func startSession(ctx context.Context, store SessionStore, userID string, ttl time.Duration) (entities.IssuedSession, error) {
	session, issued, err := entities.OpenSession(userID, clock.Now().UTC(), ttl)
	if err != nil {
		return entities.IssuedSession{}, err
	}
	if err := store.Create(ctx, session); err != nil {
		return entities.IssuedSession{}, err
	}
	return issued, nil
}

func actorOf(user entities.User) actor.Actor {
	return actor.Actor{UserID: user.ID, Login: user.Login}
}

func userEvent(a actor.Actor, action string, user entities.User) events.Event {
	return events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action,
		TargetKind: events.TargetUser, TargetID: user.ID, TargetName: user.Login,
	}
}
