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
	user, err := u.users.FindByID(ctx, t.OwnerID)
	if err != nil {
		return entities.TokenAuthenticated{}, entities.ErrTokenInvalid
	}
	return entities.TokenAuthenticated{User: user, Token: t}, u.touchIfDue(ctx, t, now, ip)
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
	user, err := u.users.FindByID(ctx, t.OwnerID)
	if err != nil {
		return actor.Actor{}, err
	}
	return entities.TokenAuthenticated{User: user, Token: t}.Actor(), nil
}
