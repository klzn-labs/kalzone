package configs_test

import (
	"context"
	"testing"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/app/envconfig/usecases/configs"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/adrianozp/platform/pkg/events"
)

type store struct {
	revisions []entities.Revision
	versions  []entities.SecretVersion
	takenHost string
}

func (s *store) Latest(_ context.Context, serviceID, env string) (entities.Revision, bool, error) {
	for i := len(s.revisions) - 1; i >= 0; i-- {
		if s.revisions[i].ServiceID == serviceID && s.revisions[i].Environment == env {
			return s.revisions[i], true, nil
		}
	}
	return entities.Revision{}, false, nil
}

func (s *store) History(context.Context, string, string, int) ([]entities.Revision, error) {
	return s.revisions, nil
}

func (s *store) Insert(_ context.Context, rev entities.Revision) error {
	s.revisions = append(s.revisions, rev)
	return nil
}

func (s *store) HostTaken(_ context.Context, host, _, _ string) (bool, error) {
	return host == s.takenHost, nil
}

func (s *store) InsertSecretVersion(_ context.Context, v entities.SecretVersion) error {
	s.versions = append(s.versions, v)
	return nil
}

func (s *store) SecretVersions(_ context.Context, ids []string) ([]entities.SecretVersion, error) {
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	result := []entities.SecretVersion{}
	for _, v := range s.versions {
		if wanted[v.ID] {
			result = append(result, v)
		}
	}
	return result, nil
}

type services struct{}

func (services) FindService(_ context.Context, slug string) (entities.ServiceRef, error) {
	if slug != "api" {
		return entities.ServiceRef{}, apperror.NotFound("service not found")
	}
	return entities.ServiceRef{ID: "s1", Slug: "api", OwnerTeamID: "t1"}, nil
}

type environments struct{}

func (environments) EnvironmentExists(_ context.Context, slug string) error {
	if slug != "dev" && slug != "prod" {
		return apperror.NotFound("environment not found")
	}
	return nil
}

type sealer struct{ additionalData [][]byte }

func (s *sealer) Seal(plaintext, additionalData []byte) ([]byte, []byte, string, error) {
	s.additionalData = append(s.additionalData, additionalData)
	return append([]byte("sealed:"), plaintext...), []byte("nonce"), "k1", nil
}

type checks struct {
	resources []authz.Resource
	denyEnv   string
}

func (c *checks) Can(_ context.Context, _ actor.Actor, _ authz.Action, r authz.Resource) error {
	c.resources = append(c.resources, r)
	if r.Environment == c.denyEnv {
		return apperror.Forbidden("no")
	}
	return nil
}

type published struct{ events []events.Event }

func (p *published) Publish(_ context.Context, e events.Event) { p.events = append(p.events, e) }

type fixture struct {
	store   *store
	sealer  *sealer
	checks  *checks
	events  *published
	usecase configs.ConfigUsecase
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	f := fixture{store: &store{}, sealer: &sealer{}, checks: &checks{}, events: &published{}}
	cfg := config.Config{AllowedDomains: []string{"kalz.one"}}
	f.usecase = configs.NewConfigUsecase(cfg, f.store, services{}, environments{}, f.sealer, f.checks, f.events)
	return f
}

var ana = actor.Actor{UserID: "ana", Login: "ana"}
