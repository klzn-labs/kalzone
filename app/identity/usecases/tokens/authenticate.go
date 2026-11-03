package tokens

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

const touchEvery = time.Minute

func (u TokensUsecase) Authenticate(ctx context.Context, raw, ip string) (entities.TokenAuthenticated, error) {
	if !entities.LooksLikeToken(raw) {
		return entities.TokenAuthenticated{}, entities.ErrTokenInvalid
	}
	t, err := u.store.FindByHash(ctx, entities.HashToken(raw))
	if err != nil {
		return entities.TokenAuthenticated{}, err
	}
	now := clock.Now().UTC()
	if err := t.Check(now); err != nil {
		return entities.TokenAuthenticated{}, err
	}
	authenticated, err := u.owned(ctx, t)
	if err != nil {
		return entities.TokenAuthenticated{}, err
	}
	return authenticated, u.touchIfDue(ctx, t, now, ip)
}

func (u TokensUsecase) owned(ctx context.Context, t entities.AccessToken) (entities.TokenAuthenticated, error) {
	user, err := u.users.FindByID(ctx, t.OwnerID)
	if err != nil || user.Disabled() {
		return entities.TokenAuthenticated{}, entities.ErrTokenInvalid
	}
	teams, err := u.scope.VisibleTeamIDs(ctx, user.ID)
	return entities.TokenAuthenticated{User: user, Teams: teams, Token: t}, err
}

func (u TokensUsecase) touchIfDue(ctx context.Context, t entities.AccessToken, now time.Time, ip string) error {
	if t.LastUsedAt != nil && now.Sub(*t.LastUsedAt) < touchEvery && t.LastUsedIP == ip {
		return nil
	}
	return u.store.Touch(ctx, t.ID, now, ip)
}

func (u TokensUsecase) ActorOf(ctx context.Context, tokenID string) (actor.Actor, error) {
	t, err := u.store.FindByID(ctx, tokenID)
	if err != nil {
		return actor.Actor{}, err
	}
	if err := t.Check(clock.Now().UTC()); err != nil {
		return actor.Actor{}, err
	}
	authenticated, err := u.owned(ctx, t)
	return authenticated.Actor(), err
}
