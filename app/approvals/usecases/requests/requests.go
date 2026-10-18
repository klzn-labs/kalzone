package requests

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/klzn-labs/kalzone/pkg/page"
)

type Store interface {
	Get(ctx context.Context, id string) (entities.Request, error)
	List(ctx context.Context, f entities.Filter) ([]entities.Request, error)
	Overdue(ctx context.Context, now time.Time) ([]entities.Request, error)
	Transition(ctx context.Context, r entities.Request, from entities.Status) error
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type TokenActors interface {
	ActorOf(ctx context.Context, tokenID string) (actor.Actor, error)
}

type Executor interface {
	Execute(ctx context.Context, a actor.Actor, payload []byte) (string, error)
	Preview(ctx context.Context, payload []byte) (approval.Preview, error)
}

type Executors map[string]Executor

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

const listLimit = 100

var (
	errNotVisible = apperror.NotFound("change request not found").WithHint("list change requests with GET /api/v1/requests")
	errStatus     = apperror.Invalid("unknown status").WithField("status").
			WithHint("send one of " + statusList() + ", or leave status out")
	errNotPending = apperror.Conflict("this request was already decided")
	errExpired    = apperror.Conflict("this request expired, ask for a new one")
	errNotYours   = apperror.Forbidden("only the person who asked can cancel a request, approvers reject it")
	errNoExecutor = apperror.Invalid("the platform does not know how to run this kind of request")
)

type RequestsUsecase struct {
	store     Store
	authz     Authorizer
	tokens    TokenActors
	executors Executors
	publisher Publisher
}

func NewRequestsUsecase(store Store, authz Authorizer, tokens TokenActors, executors Executors, publisher Publisher) RequestsUsecase {
	return RequestsUsecase{store: store, authz: authz, tokens: tokens, executors: executors, publisher: publisher}
}

func (u RequestsUsecase) Get(ctx context.Context, a actor.Actor, id string) (entities.Request, error) {
	r, err := u.store.Get(ctx, id)
	if err != nil {
		return entities.Request{}, err
	}
	if !a.Sees(r.TeamID) {
		return entities.Request{}, errNotVisible
	}
	return r, nil
}

func (u RequestsUsecase) RequestState(ctx context.Context, a actor.Actor, id string) (string, bool, error) {
	r, err := u.Get(ctx, a, id)
	return string(r.Status), r.Final(), err
}

func (u RequestsUsecase) List(ctx context.Context, a actor.Actor, status entities.Status) ([]entities.Request, error) {
	if a.ViaToken() && len(a.Token.TeamIDs) == 0 {
		return []entities.Request{}, nil
	}
	return u.store.List(ctx, entities.Filter{Status: status, TeamIDs: a.Token.TeamIDs, Limit: listLimit})
}

func (u RequestsUsecase) PageFor(ctx context.Context, a actor.Actor, status string, p page.Request) (page.Page[entities.Request], error) {
	filter, err := filterOf(a, status, p)
	if err != nil {
		return page.Page[entities.Request]{}, err
	}
	if a.ViaToken() && len(a.Token.TeamIDs) == 0 {
		return page.Page[entities.Request]{Items: []entities.Request{}}, nil
	}
	list, err := u.store.List(ctx, filter)
	return page.Cut(list, p, requestKey), err
}

func filterOf(a actor.Actor, status string, p page.Request) (entities.Filter, error) {
	if status != "" && !slices.Contains(entities.Statuses, entities.Status(status)) {
		return entities.Filter{}, errStatus
	}
	before, beforeID, err := p.Time()
	filter := entities.Filter{Status: entities.Status(status), TeamIDs: a.Token.TeamIDs, Before: before, BeforeID: beforeID, Limit: p.Fetch()}
	return filter, err
}

func statusList() string {
	names := make([]string, 0, len(entities.Statuses))
	for _, s := range entities.Statuses {
		names = append(names, string(s))
	}
	return strings.Join(names, ", ")
}

func requestKey(r entities.Request) string {
	return page.TimeKey(r.CreatedAt, r.ID)
}

func (u RequestsUsecase) Awaiting(ctx context.Context, a actor.Actor) ([]entities.Request, error) {
	pending, err := u.List(ctx, a, entities.StatusPending)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(pending, func(r entities.Request) bool { return !u.MayDecide(ctx, a, r) }), nil
}

func (u RequestsUsecase) MayDecide(ctx context.Context, a actor.Actor, r entities.Request) bool {
	return u.authz.Can(ctx, a, authz.ApproveRequest, authz.Resource{TeamID: r.TeamID}) == nil
}

func (u RequestsUsecase) Preview(ctx context.Context, r entities.Request) (approval.Preview, error) {
	executor, known := u.executors[r.Kind]
	if !known {
		return approval.Preview{}, errNoExecutor
	}
	return executor.Preview(ctx, r.Payload)
}
