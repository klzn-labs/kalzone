package requests_test

import (
	"context"
	"time"

	"github.com/adrianozp/platform/app/approvals/entities"
	"github.com/adrianozp/platform/app/approvals/usecases/requests"
	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/approval"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/events"
)

var (
	ctx = context.Background()
	now = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	mia = actor.Actor{UserID: "u-mia", Login: "mia"}
	bob = actor.Actor{UserID: "u-bob", Login: "bob"}
)

type store struct {
	byID   map[string]entities.Request
	broken error
}

func (s *store) Get(_ context.Context, id string) (entities.Request, error) {
	r, ok := s.byID[id]
	if !ok {
		return entities.Request{}, apperror.NotFound("change request not found")
	}
	return r, nil
}

func (s *store) List(_ context.Context, f entities.Filter) ([]entities.Request, error) {
	found := []entities.Request{}
	for _, r := range s.byID {
		if (f.Status == "" || r.Status == f.Status) && (f.ServiceID == "" || r.ServiceID == f.ServiceID) {
			found = append(found, r)
		}
	}
	return found, nil
}

func (s *store) Overdue(_ context.Context, at time.Time) ([]entities.Request, error) {
	found := []entities.Request{}
	for _, r := range s.byID {
		if r.ExpiredAt(at) || (r.Status == entities.StatusApproved && !r.DecidedAt.After(at.Add(-entities.ExecutionLimit))) {
			found = append(found, r)
		}
	}
	return found, nil
}

func (s *store) Transition(_ context.Context, r entities.Request, from entities.Status) error {
	if s.broken != nil {
		return s.broken
	}
	if s.byID[r.ID].Status != from {
		return apperror.Conflict("this request was already decided")
	}
	s.byID[r.ID] = r
	return nil
}

type staleStore struct {
	*store
	seen map[string]entities.Request
}

func readingStale(s *store) staleStore {
	seen := map[string]entities.Request{}
	for id, r := range s.byID {
		seen[id] = r
	}
	return staleStore{store: s, seen: seen}
}

func (s staleStore) Get(_ context.Context, id string) (entities.Request, error) {
	return s.seen[id], nil
}

type maintainers map[string]string

func (m maintainers) Can(_ context.Context, a actor.Actor, _ authz.Action, r authz.Resource) error {
	if a.ViaToken() || m[a.UserID] != r.TeamID {
		return apperror.Forbidden("no")
	}
	return nil
}

type tokens struct {
	err   error
	asked []string
}

func (t *tokens) ActorOf(_ context.Context, tokenID string) (actor.Actor, error) {
	t.asked = append(t.asked, tokenID)
	return actor.Actor{UserID: "u-bob", Login: "bob", Token: actor.Token{ID: tokenID, Name: "claude-local"}}, t.err
}

type executor struct {
	ran    []actor.Actor
	ctxErr []error
	result string
	err    error
}

func (e *executor) Execute(ctx context.Context, a actor.Actor, _ []byte) (string, error) {
	e.ran = append(e.ran, a)
	e.ctxErr = append(e.ctxErr, ctx.Err())
	return e.result, e.err
}

func (e *executor) Preview(context.Context, []byte) (approval.Preview, error) {
	return approval.Preview{Lines: []approval.Line{{Key: "release", Before: "#1", After: "#2"}}}, nil
}

type recorder struct{ actions []string }

func (r *recorder) Publish(_ context.Context, e events.Event) {
	r.actions = append(r.actions, e.Action)
}

type fixture struct {
	store    *store
	executor *executor
	events   *recorder
	tokens   *tokens
}

func newFixture() fixture {
	clock.WithTime(now)
	f := fixture{store: &store{byID: map[string]entities.Request{}}, executor: &executor{result: "deployment #3"}, events: &recorder{}, tokens: &tokens{}}
	f.store.byID["c1"] = pendingRequest("c1", "k1")
	return f
}

func pendingRequest(id, tokenID string) entities.Request {
	return entities.Request{
		ID: id, Kind: approval.KindDeploy, TeamID: "t1", RequestedBy: "u-bob", TokenID: tokenID,
		Status: entities.StatusPending, CreatedAt: now, ExpiresAt: now.Add(entities.Lifetime),
	}
}

func (f fixture) usecase() requests.RequestsUsecase {
	return f.usecaseOver(f.store)
}

func (f fixture) usecaseOver(s requests.Store) requests.RequestsUsecase {
	return requests.NewRequestsUsecase(s, maintainers{"u-mia": "t1"}, f.tokens, requests.Executors{approval.KindDeploy: f.executor}, f.events)
}
