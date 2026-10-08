package queues_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/app/queue/usecases/queues"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/events"
)

var ctx = context.Background()

var ana = actor.Actor{UserID: "u-ana", Login: "ana"}

type store struct {
	queues   map[string]entities.Queue
	versions []entities.Version
	commands []entities.Command
}

func key(env, name string) string { return env + "/" + name }

func (s *store) Create(_ context.Context, q entities.Queue, v entities.Version) error {
	if _, taken := s.queues[key(q.Environment, q.Name)]; taken {
		return apperror.Conflict("taken")
	}
	s.queues[key(q.Environment, q.Name)] = q
	s.versions = append(s.versions, v)
	return nil
}

func (s *store) InsertVersion(_ context.Context, q entities.Queue, v entities.Version) error {
	s.queues[key(q.Environment, q.Name)] = q
	s.versions = append(s.versions, v)
	return nil
}

func (s *store) SetPaused(_ context.Context, id string, paused bool) error {
	for k, q := range s.queues {
		if q.ID == id {
			q.Paused = paused
			s.queues[k] = q
		}
	}
	return nil
}

func (s *store) Get(_ context.Context, env, name string) (entities.Queue, error) {
	q, ok := s.queues[key(env, name)]
	if !ok {
		return entities.Queue{}, apperror.NotFound("queue not found")
	}
	return q, nil
}

func (s *store) List(context.Context, string, string) ([]entities.Queue, error) {
	result := []entities.Queue{}
	for _, q := range s.queues {
		result = append(result, q)
	}
	return result, nil
}

func (s *store) Delete(_ context.Context, id string, deletion entities.Command) error {
	s.commands = append(s.commands, deletion)
	for k, q := range s.queues {
		if q.ID == id {
			delete(s.queues, k)
		}
	}
	return nil
}

func (s *store) Versions(context.Context, string, int) ([]entities.Version, error) {
	return s.versions, nil
}

func (s *store) AddCommand(_ context.Context, c entities.Command) error {
	s.commands = append(s.commands, c)
	return nil
}

var services = map[string]queues.ServiceRef{
	"worker":   {ID: "s-worker", Slug: "worker", OwnerTeamID: "t-plateful", Port: 8081},
	"importer": {ID: "s-importer", Slug: "importer", OwnerTeamID: "t-plateful", Port: 8082},
}

type lookup struct{}

func (lookup) FindService(_ context.Context, slug string) (queues.ServiceRef, error) {
	s, ok := services[slug]
	if !ok {
		return queues.ServiceRef{}, apperror.NotFound("service not found")
	}
	return s, nil
}

func (lookup) FindServiceByID(_ context.Context, id string) (queues.ServiceRef, error) {
	for _, s := range services {
		if s.ID == id {
			return s, nil
		}
	}
	return queues.ServiceRef{}, apperror.NotFound("service not found")
}

func (lookup) EnvironmentExists(_ context.Context, slug string) error {
	if slug == "dev" || slug == "prod" {
		return nil
	}
	return apperror.NotFound("environment not found")
}

func (lookup) FindTeam(_ context.Context, slug string) (string, error) {
	return "t-" + slug, nil
}

type stats map[string]entities.Stats

func (s stats) Latest(id string) (entities.Stats, bool) {
	st, ok := s[id]
	return st, ok
}

type policy struct{ deny bool }

func (p policy) Can(context.Context, actor.Actor, authz.Action, authz.Resource) error {
	if p.deny {
		return apperror.Forbidden("no")
	}
	return nil
}

type recorder struct{ actions []string }

func (r *recorder) Publish(_ context.Context, e events.Event) {
	r.actions = append(r.actions, e.Action)
}

type fixture struct {
	usecase queues.QueuesUsecase
	store   *store
	stats   stats
	events  *recorder
}

func newFixture(t *testing.T, deny bool) fixture {
	t.Helper()
	s := &store{queues: map[string]entities.Queue{}}
	st := stats{}
	r := &recorder{}
	u := queues.NewQueuesUsecase(s, lookup{}, lookup{}, lookup{}, st, policy{deny: deny}, r)
	return fixture{usecase: u, store: s, stats: st, events: r}
}

func draft() queues.Draft {
	return queues.Draft{Name: "place-import", Environment: "prod", Consumer: "worker", Publishers: []string{"importer"}, Settings: entities.Settings{Path: "/hooks/place-import"}}
}

func fresh(depth int64) entities.Stats {
	return entities.Stats{Depth: depth, ReportedAt: time.Now().UTC()}
}
