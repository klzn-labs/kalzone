package crons_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/app/cron/usecases/crons"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/events"
)

var ctx = context.Background()

var ana = actor.Actor{UserID: "u-ana", Teams: []string{"t-plateful"}, Login: "ana"}

type store struct {
	crons    map[string]entities.Cron
	commands []entities.Command
	runs     []entities.Run
}

func key(env, name string) string { return env + "/" + name }

func (s *store) Create(_ context.Context, c entities.Cron) error {
	if _, taken := s.crons[key(c.Environment, c.Name)]; taken {
		return apperror.Conflict("taken")
	}
	s.crons[key(c.Environment, c.Name)] = c
	return nil
}

func (s *store) Save(_ context.Context, c entities.Cron) error {
	s.crons[key(c.Environment, c.Name)] = c
	return nil
}

func (s *store) SetPaused(_ context.Context, id string, paused bool) error {
	for k, c := range s.crons {
		if c.ID == id {
			c.Paused = paused
			s.crons[k] = c
		}
	}
	return nil
}

func (s *store) Get(_ context.Context, env, name string) (entities.Cron, error) {
	c, ok := s.crons[key(env, name)]
	if !ok {
		return entities.Cron{}, apperror.NotFound("cron not found")
	}
	return c, nil
}

func (s *store) List(_ context.Context, env, _ string) ([]entities.Cron, error) {
	result := []entities.Cron{}
	for _, c := range s.crons {
		if env == "" || c.Environment == env {
			result = append(result, c)
		}
	}
	return result, nil
}

func (s *store) ForService(_ context.Context, serviceID string) ([]entities.Cron, error) {
	result := []entities.Cron{}
	for _, c := range s.crons {
		if c.ServiceID == serviceID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (s *store) Delete(_ context.Context, id string) error {
	for k, c := range s.crons {
		if c.ID == id {
			delete(s.crons, k)
		}
	}
	return nil
}

func (s *store) AddCommand(_ context.Context, c entities.Command) error {
	s.commands = append(s.commands, c)
	return nil
}

func (s *store) Runs(_ context.Context, cronID string, limit int) ([]entities.Run, error) {
	result := []entities.Run{}
	for _, r := range s.runs {
		if r.CronID == cronID && len(result) < limit {
			result = append(result, r)
		}
	}
	return result, nil
}

var services = map[string]crons.ServiceRef{
	"worker":  {ID: "s-worker", Slug: "worker", OwnerTeamID: "t-plateful"},
	"leaving": {ID: "s-leaving", Slug: "leaving", OwnerTeamID: "t-plateful", Decommissioning: true},
}

type catalog struct{}

func (catalog) FindService(_ context.Context, slug string) (crons.ServiceRef, error) {
	s, ok := services[slug]
	if !ok {
		return crons.ServiceRef{}, apperror.NotFound("service not found")
	}
	return s, nil
}

func (catalog) FindServiceByID(_ context.Context, id string) (crons.ServiceRef, error) {
	for _, s := range services {
		if s.ID == id {
			return s, nil
		}
	}
	return crons.ServiceRef{}, apperror.NotFound("service not found")
}

func (catalog) EnvironmentExists(_ context.Context, slug string) error {
	if slug == "dev" || slug == "prod" {
		return nil
	}
	return apperror.NotFound("environment not found")
}

func (catalog) FindTeam(_ context.Context, slug string) (string, error) {
	return "t-" + slug, nil
}

type policy struct {
	deny bool
	gate authz.Gate
}

func (p policy) Gate(context.Context, actor.Actor, authz.Action, authz.Resource) (authz.Gate, error) {
	if p.deny {
		return "", apperror.Forbidden("no")
	}
	if p.gate == "" {
		return authz.Open, nil
	}
	return p.gate, nil
}

type requests struct{ changes []approval.Change }

func (r *requests) Submit(_ context.Context, _ actor.Actor, c approval.Change) (*approval.Pending, error) {
	r.changes = append(r.changes, c)
	return &approval.Pending{ID: "c1", Status: "pending"}, nil
}

type recorder struct{ events []events.Event }

func (r *recorder) Publish(_ context.Context, e events.Event) {
	r.events = append(r.events, e)
}

func (r *recorder) actions() []string {
	result := []string{}
	for _, e := range r.events {
		result = append(result, e.Action)
	}
	return result
}

type fixture struct {
	usecase  crons.CronsUsecase
	store    *store
	events   *recorder
	requests *requests
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	f := fixture{store: &store{crons: map[string]entities.Cron{}}, events: &recorder{}, requests: &requests{}}
	f.usecase = f.as(authz.Open)
	return f
}

func (f fixture) as(gate authz.Gate) crons.CronsUsecase {
	return crons.NewCronsUsecase(f.store, catalog{}, policy{gate: gate}, f.events, f.requests)
}

func (f fixture) denied() crons.CronsUsecase {
	return crons.NewCronsUsecase(f.store, catalog{}, policy{deny: true}, f.events, f.requests)
}

func draft() crons.Draft {
	return crons.Draft{Name: "nightly-import", Environment: "prod", Service: "worker", Schedule: "0 3 * * *", Path: "/hooks/import"}
}

func ptr[T any](v T) *T { return &v }

var minute = time.Minute
