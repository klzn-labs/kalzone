package releases_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/app/release/usecases/releases"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/events"
)

type store struct{ byID map[string]entities.Release }

func (s *store) Create(_ context.Context, rel entities.Release) (entities.Release, error) {
	rel.Number = len(s.byID) + 1
	s.byID[rel.ID] = rel
	return rel, nil
}

func (s *store) Transition(_ context.Context, rel entities.Release, from entities.Status) error {
	if s.byID[rel.ID].Status != from {
		return apperror.Conflict("changed")
	}
	s.byID[rel.ID] = rel
	return nil
}

func (s *store) FindByID(_ context.Context, id string) (entities.Release, error) {
	rel, ok := s.byID[id]
	if !ok {
		return entities.Release{}, apperror.NotFound("release not found").WithHint("list a service's releases with GET /api/v1/releases?service=<slug>")
	}
	return rel, nil
}

func (s *store) ListByService(_ context.Context, serviceID string, before, limit int) ([]entities.Release, error) {
	result := []entities.Release{}
	for _, rel := range s.byID {
		if rel.ServiceID == serviceID && (before == 0 || rel.Number < before) {
			result = append(result, rel)
		}
	}
	slices.SortFunc(result, func(a, b entities.Release) int { return b.Number - a.Number })
	return result[:min(limit, len(result))], nil
}

func (s *store) Unfinished(_ context.Context, before time.Time) ([]entities.Release, error) {
	result := []entities.Release{}
	for _, rel := range s.byID {
		if !rel.Final() && rel.CreatedAt.Before(before) {
			result = append(result, rel)
		}
	}
	return result, nil
}

func (s *store) UnfinishedFor(_ context.Context, serviceID string) ([]entities.Release, error) {
	result := []entities.Release{}
	for _, rel := range s.byID {
		if !rel.Final() && rel.ServiceID == serviceID {
			result = append(result, rel)
		}
	}
	return result, nil
}

func (s *store) LatestBuilt(_ context.Context, serviceID string) (entities.Release, bool, error) {
	return s.newest(func(r entities.Release) bool { return r.ServiceID == serviceID && r.Status == entities.StatusBuilt })
}

func (s *store) ByNumber(_ context.Context, serviceID string, number int) (entities.Release, bool, error) {
	return s.newest(func(r entities.Release) bool { return r.ServiceID == serviceID && r.Number == number })
}

func (s *store) BuiltByCommitPrefix(_ context.Context, serviceID, prefix string, limit int) ([]entities.Release, error) {
	found := s.sorted(func(r entities.Release) bool {
		return r.ServiceID == serviceID && r.Status == entities.StatusBuilt && strings.HasPrefix(r.CommitSHA, prefix)
	})
	return found[:min(limit, len(found))], nil
}

func (s *store) BuiltByTag(_ context.Context, serviceID, tag string) (entities.Release, bool, error) {
	return s.newest(func(r entities.Release) bool {
		return r.ServiceID == serviceID && r.Status == entities.StatusBuilt && r.Tag == tag
	})
}

func (s *store) newest(keep func(entities.Release) bool) (entities.Release, bool, error) {
	found := s.sorted(keep)
	if len(found) == 0 {
		return entities.Release{}, false, nil
	}
	return found[0], true, nil
}

func (s *store) sorted(keep func(entities.Release) bool) []entities.Release {
	result := []entities.Release{}
	for _, rel := range s.byID {
		if keep(rel) {
			result = append(result, rel)
		}
	}
	slices.SortFunc(result, func(a, b entities.Release) int { return b.Number - a.Number })
	return result
}

var api = entities.ServiceRef{ID: "s1", Slug: "api", OwnerTeamID: "t1", GitHubRepo: "o/api", ImageRepository: "docker.io/o/api", BuildWorkflow: "platform-build.yml", AutoBuild: true}
var web = entities.ServiceRef{ID: "s2", Slug: "web", OwnerTeamID: "t1", GitHubRepo: "o/api", ImageRepository: "docker.io/o/web", BuildWorkflow: "platform-build.yml"}
var worker = entities.ServiceRef{ID: "s4", Slug: "worker", OwnerTeamID: "t1", GitHubRepo: "o/api", ImageRepository: "docker.io/o/api", BuildWorkflow: "platform-build.yml", AutoBuild: true}
var bare = entities.ServiceRef{ID: "s3", Slug: "bare", OwnerTeamID: "t1", BuildWorkflow: "platform-build.yml"}
var retired = entities.ServiceRef{
	ID: "s5", Slug: "retired", OwnerTeamID: "t1", GitHubRepo: "o/retired", ImageRepository: "docker.io/o/retired",
	BuildWorkflow: "platform-build.yml", AutoBuild: true, Decommissioning: true,
}

type services struct{}

func (services) FindService(_ context.Context, slug string) (entities.ServiceRef, error) {
	return byKey(func(s entities.ServiceRef) bool { return s.Slug == slug })
}

func (services) FindServiceByID(_ context.Context, id string) (entities.ServiceRef, error) {
	return byKey(func(s entities.ServiceRef) bool { return s.ID == id })
}

func (services) ServicesForRepository(_ context.Context, repo string) ([]entities.ServiceRef, error) {
	if repo == retired.GitHubRepo {
		return []entities.ServiceRef{retired}, nil
	}
	return []entities.ServiceRef{api, web, worker}, nil
}

func byKey(match func(entities.ServiceRef) bool) (entities.ServiceRef, error) {
	for _, s := range []entities.ServiceRef{api, web, worker, bare, retired} {
		if match(s) {
			return s, nil
		}
	}
	return entities.ServiceRef{}, apperror.NotFound("service not found").WithHint("list services with GET /api/v1/services")
}

type builder struct {
	requests []entities.BuildRequest
	err      error
}

func (b *builder) Dispatch(_ context.Context, r entities.BuildRequest) error {
	b.requests = append(b.requests, r)
	return b.err
}

type resolver struct{}

func (resolver) Resolve(_ context.Context, image, tag string) (string, error) {
	if tag == "missing" {
		return "", apperror.NotFound("tag not found")
	}
	return "sha256:" + digestHex, nil
}

const digestHex = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type allow struct{ deny bool }

func (a allow) Can(context.Context, actor.Actor, authz.Action, authz.Resource) error {
	if a.deny {
		return apperror.Forbidden("no")
	}
	return nil
}

type published struct{ actions []string }

func (p *published) Publish(_ context.Context, e events.Event) {
	p.actions = append(p.actions, e.Action)
}

type fixture struct {
	store   *store
	builder *builder
	events  *published
	usecase releases.ReleasesUsecase
}

func newFixture(t *testing.T, deny bool) fixture {
	t.Helper()
	f := fixture{store: &store{byID: map[string]entities.Release{}}, builder: &builder{}, events: &published{}}
	cfg := config.Config{BaseURL: "https://platform.test", Build: config.Build{Timeout: time.Hour}}
	f.usecase = releases.NewReleasesUsecase(cfg, f.store, services{}, f.builder, resolver{}, allow{deny}, f.events)
	return f
}

var ana = actor.Actor{UserID: "ana", Teams: []string{"t1"}, Login: "ana"}
var errBoom = errors.New("422 workflow not found")
