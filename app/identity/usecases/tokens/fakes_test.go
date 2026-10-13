package tokens_test

import (
	"context"
	"time"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/identity/entities"
	"github.com/adrianozp/platform/app/identity/usecases/tokens"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/events"
	"github.com/adrianozp/platform/pkg/scope"
)

var (
	ctx = context.Background()
	now = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	ana = actor.Actor{UserID: "u-ana", Login: "ana"}
	ops = actor.Actor{UserID: "u-ops", Login: "ops", Admin: true}
)

type store struct {
	byID    map[string]entities.AccessToken
	touched int
}

func (s *store) Create(_ context.Context, t entities.AccessToken) error {
	for _, other := range s.byID {
		if other.OwnerID == t.OwnerID && other.Name == t.Name && other.RevokedAt == nil {
			return apperror.Conflict("taken")
		}
	}
	s.byID[t.ID] = t
	return nil
}

func (s *store) FindByHash(_ context.Context, hash string) (entities.AccessToken, error) {
	for _, t := range s.byID {
		if t.Hash == hash {
			return t, nil
		}
	}
	return entities.AccessToken{}, entities.ErrTokenInvalid
}

func (s *store) FindByID(_ context.Context, id string) (entities.AccessToken, error) {
	t, ok := s.byID[id]
	if !ok {
		return entities.AccessToken{}, apperror.NotFound("token not found")
	}
	return t, nil
}

func (s *store) ListByOwner(_ context.Context, ownerID string) ([]entities.AccessToken, error) {
	result := []entities.AccessToken{}
	for _, t := range s.byID {
		if t.OwnerID == ownerID {
			result = append(result, t)
		}
	}
	return result, nil
}

func (s *store) ListAll(context.Context) ([]entities.AccessToken, error) {
	result := []entities.AccessToken{}
	for _, t := range s.byID {
		result = append(result, t)
	}
	return result, nil
}

func (s *store) Revoke(_ context.Context, id string, at time.Time) error {
	t := s.byID[id]
	t.RevokedAt = &at
	s.byID[id] = t
	return nil
}

func (s *store) Touch(_ context.Context, id string, at time.Time, ip string) error {
	t := s.byID[id]
	t.LastUsedAt, t.LastUsedIP = &at, ip
	s.byID[id] = t
	s.touched++
	return nil
}

type users struct{}

func (users) FindByID(_ context.Context, id string) (entities.User, error) {
	return entities.User{ID: id, Login: id[2:]}, nil
}

type teams map[string]string

func (t teams) IDBySlug(_ context.Context, slug string) (string, error) {
	id, ok := t[slug]
	if !ok {
		return "", apperror.NotFound("team not found")
	}
	return id, nil
}

func (t teams) SlugsByIDs(_ context.Context, ids []string) ([]string, error) {
	result := []string{}
	for slug, id := range t {
		if contains(ids, id) {
			result = append(result, slug)
		}
	}
	return result, nil
}

func (t teams) SlugByID(_ context.Context, ids []string) (map[string]string, error) {
	result := map[string]string{}
	for slug, id := range t {
		if contains(ids, id) {
			result[id] = slug
		}
	}
	return result, nil
}

func contains(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

type allow struct{}

func (allow) Can(_ context.Context, a actor.Actor, _ authz.Action, _ authz.Resource) error {
	if a.ViaToken() && !a.Token.Has(scope.TokensAdmin) {
		return apperror.MissingScope(string(scope.TokensAdmin))
	}
	return nil
}

type recorder struct{ actions []string }

func (r *recorder) Publish(_ context.Context, e events.Event) {
	r.actions = append(r.actions, e.Action)
}

type fixture struct {
	usecase tokens.TokensUsecase
	store   *store
	events  *recorder
}

func newFixture() fixture {
	clock.WithTime(now)
	s := &store{byID: map[string]entities.AccessToken{}}
	r := &recorder{}
	u := tokens.NewTokensUsecase(s, users{}, teams{"places": "t1", "reviews": "t2"}, allow{}, r)
	return fixture{usecase: u, store: s, events: r}
}

func agentOf(owner actor.Actor, scopes []scope.Scope, teamIDs []string, days int) actor.Actor {
	owner.Token = actor.Token{ID: "parent", Name: "parent", Scopes: scopes, TeamIDs: teamIDs, ExpiresAt: now.Add(time.Duration(days) * 24 * time.Hour)}
	return owner
}

func draftOf(scopes ...string) entities.TokenDraft {
	return entities.TokenDraft{Name: "claude-local", Scopes: scopes, Teams: []string{"places"}}
}

func lasting(days int, d entities.TokenDraft) entities.TokenDraft {
	d.ExpiresInDays = days
	return d
}
