package tokens

import (
	"context"
	"slices"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/identity/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/uuid"
)

var (
	errBypassNeedsAdmin = apperror.Forbidden("only a platform admin in a browser session can grant bypass")
	errEscalation       = apperror.Forbidden("a token can only create tokens with its own scopes and teams, or fewer")
)

func (u TokensUsecase) Create(ctx context.Context, a actor.Actor, d entities.TokenDraft) (entities.IssuedToken, error) {
	if err := u.authz.Can(ctx, a, authz.ManageTokens, authz.Resource{}); err != nil {
		return entities.IssuedToken{}, err
	}
	grant, err := u.grantOf(ctx, d)
	if err != nil {
		return entities.IssuedToken{}, err
	}
	if grant, err = fitGrant(a, grant); err != nil {
		return entities.IssuedToken{}, err
	}
	issued, err := newIssued(a, grant)
	if err != nil {
		return entities.IssuedToken{}, err
	}
	return u.save(ctx, a, issued)
}

func (u TokensUsecase) save(ctx context.Context, a actor.Actor, issued entities.IssuedToken) (entities.IssuedToken, error) {
	slugs, err := u.teams.SlugsByIDs(ctx, issued.Token.TeamIDs)
	if err != nil {
		return entities.IssuedToken{}, err
	}
	if err := u.store.Create(ctx, issued.Token); err != nil {
		return entities.IssuedToken{}, err
	}
	u.publish(ctx, a, "token.created", issued.Token)
	issued.Token.TeamSlugs = slugs
	return issued, nil
}

func (u TokensUsecase) grantOf(ctx context.Context, d entities.TokenDraft) (entities.TokenGrant, error) {
	scopes, err := d.Check()
	if err != nil {
		return entities.TokenGrant{}, err
	}
	teamIDs, err := u.teamIDs(ctx, d.Teams)
	if err != nil {
		return entities.TokenGrant{}, err
	}
	expiresAt := d.ExpiresAt(clock.Now().UTC())
	return entities.TokenGrant{Name: d.Name, Scopes: scopes, TeamIDs: teamIDs, Bypass: d.Bypass, ExpiresAt: expiresAt}, nil
}

func (u TokensUsecase) teamIDs(ctx context.Context, slugs []string) ([]string, error) {
	ids := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		id, err := u.teams.IDBySlug(ctx, slug)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func fitGrant(a actor.Actor, g entities.TokenGrant) (entities.TokenGrant, error) {
	if g.Bypass && !a.SessionAdmin() {
		return g, errBypassNeedsAdmin
	}
	if !a.ViaToken() {
		return g, nil
	}
	if !subset(g.Scopes, a.Token.Scopes) || !subset(g.TeamIDs, a.Token.TeamIDs) {
		return g, errEscalation
	}
	if g.ExpiresAt.After(a.Token.ExpiresAt) {
		g.ExpiresAt = a.Token.ExpiresAt
	}
	return g, nil
}

func subset[T comparable](inner, outer []T) bool {
	for _, item := range inner {
		if !slices.Contains(outer, item) {
			return false
		}
	}
	return true
}

func newIssued(a actor.Actor, g entities.TokenGrant) (entities.IssuedToken, error) {
	secret, err := entities.NewTokenSecret()
	if err != nil {
		return entities.IssuedToken{}, err
	}
	token := entities.AccessToken{
		ID: uuid.NewUUID(), OwnerID: a.UserID, Name: g.Name, Hash: secret.Hash, Prefix: secret.Prefix,
		Scopes: g.Scopes, TeamIDs: g.TeamIDs, Bypass: g.Bypass, CreatedAt: clock.Now().UTC(),
		CreatedByTokenID: a.Token.ID, ExpiresAt: g.ExpiresAt,
	}
	return entities.IssuedToken{Token: token, Secret: secret.Raw}, nil
}
