package tokens

import (
	"context"
	"time"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/identity/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/events"
)

type TokenStore interface {
	Create(ctx context.Context, t entities.AccessToken) error
	FindByHash(ctx context.Context, hash string) (entities.AccessToken, error)
	FindByID(ctx context.Context, id string) (entities.AccessToken, error)
	ListByOwner(ctx context.Context, ownerID string) ([]entities.AccessToken, error)
	ListAll(ctx context.Context) ([]entities.AccessToken, error)
	Revoke(ctx context.Context, id string, at time.Time) error
	Touch(ctx context.Context, id string, at time.Time, ip string) error
}

type UserReader interface {
	FindByID(ctx context.Context, id string) (entities.User, error)
}

type TeamLookup interface {
	IDBySlug(ctx context.Context, slug string) (string, error)
	SlugsByIDs(ctx context.Context, ids []string) ([]string, error)
	SlugByID(ctx context.Context, ids []string) (map[string]string, error)
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

var (
	errOnlyAdmins    = apperror.Forbidden("only a platform admin in a browser session sees everyone's tokens")
	errTokenNotFound = apperror.NotFound("token not found")
)

type TokensUsecase struct {
	store     TokenStore
	users     UserReader
	teams     TeamLookup
	authz     Authorizer
	publisher Publisher
}

func NewTokensUsecase(store TokenStore, users UserReader, teams TeamLookup, authz Authorizer, publisher Publisher) TokensUsecase {
	return TokensUsecase{store: store, users: users, teams: teams, authz: authz, publisher: publisher}
}

func (u TokensUsecase) List(ctx context.Context, a actor.Actor) ([]entities.AccessToken, error) {
	if err := u.authz.Can(ctx, a, authz.ManageTokens, authz.Resource{}); err != nil {
		return nil, err
	}
	list, err := u.store.ListByOwner(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	return u.withTeams(ctx, list)
}

func (u TokensUsecase) withTeams(ctx context.Context, list []entities.AccessToken) ([]entities.AccessToken, error) {
	slugByID, err := u.teams.SlugByID(ctx, teamIDsOf(list))
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].TeamSlugs = slugsOf(list[i].TeamIDs, slugByID)
	}
	return list, nil
}

func teamIDsOf(list []entities.AccessToken) []string {
	seen := map[string]bool{}
	ids := []string{}
	for _, t := range list {
		for _, id := range t.TeamIDs {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func slugsOf(ids []string, slugByID map[string]string) []string {
	slugs := []string{}
	for _, id := range ids {
		if slug, ok := slugByID[id]; ok {
			slugs = append(slugs, slug)
		}
	}
	return slugs
}

func (u TokensUsecase) ListAll(ctx context.Context, a actor.Actor) ([]entities.AccessToken, error) {
	if !a.SessionAdmin() {
		return nil, errOnlyAdmins
	}
	list, err := u.store.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return u.withTeams(ctx, list)
}

func (u TokensUsecase) Revoke(ctx context.Context, a actor.Actor, id string) error {
	if err := u.authz.Can(ctx, a, authz.ManageTokens, authz.Resource{}); err != nil {
		return err
	}
	t, err := u.store.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if t.OwnerID != a.UserID && !a.SessionAdmin() {
		return errTokenNotFound
	}
	if t.RevokedAt != nil {
		return nil
	}
	if err := u.store.Revoke(ctx, id, clock.Now().UTC()); err != nil {
		return err
	}
	u.publish(ctx, a, "token.revoked", t)
	return nil
}

func (u TokensUsecase) Whoami(ctx context.Context, a actor.Actor) (entities.Whoami, error) {
	user, err := u.users.FindByID(ctx, a.UserID)
	if err != nil {
		return entities.Whoami{}, err
	}
	teams, err := u.teams.SlugsByIDs(ctx, a.Token.TeamIDs)
	return entities.Whoami{User: user, Token: a.Token, Teams: teams}, err
}

func (u TokensUsecase) publish(ctx context.Context, a actor.Actor, action string, t entities.AccessToken) {
	u.publisher.Publish(ctx, events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action,
		TargetKind: events.TargetToken, TargetID: t.ID, TargetName: t.Name,
		Details: map[string]any{"owner_id": t.OwnerID, "scopes": t.Scopes, "bypass": t.Bypass},
	})
}
