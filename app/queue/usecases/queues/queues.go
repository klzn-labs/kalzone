package queues

import (
	"context"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/events"
)

type ServiceRef struct {
	ID          string
	Slug        string
	OwnerTeamID string
	Port        int
}

type Store interface {
	Create(ctx context.Context, q entities.Queue, v entities.Version) error
	InsertVersion(ctx context.Context, q entities.Queue, v entities.Version) error
	SetPaused(ctx context.Context, queueID string, paused bool) error
	Get(ctx context.Context, environment, name string) (entities.Queue, error)
	List(ctx context.Context, environment, teamID string) ([]entities.Queue, error)
	Delete(ctx context.Context, queueID string, deletion entities.Command) error
	Versions(ctx context.Context, queueID string, before, limit int) ([]entities.Version, error)
	AddCommand(ctx context.Context, c entities.Command) error
	DeleteCredentials(ctx context.Context, serviceID string) error
}

type ServiceLookup interface {
	FindService(ctx context.Context, slug string) (ServiceRef, error)
	FindServiceByID(ctx context.Context, id string) (ServiceRef, error)
}

type EnvironmentLookup interface {
	EnvironmentExists(ctx context.Context, slug string) error
}

type TeamLookup interface {
	FindTeam(ctx context.Context, slug string) (string, error)
}

type StatsReader interface {
	Latest(queueID string) (entities.Stats, bool)
}

type Authorizer interface {
	Gate(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) (authz.Gate, error)
}

type Requests interface {
	Submit(ctx context.Context, a actor.Actor, c approval.Change) (*approval.Pending, error)
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

type QueuesUsecase struct {
	store        Store
	services     ServiceLookup
	environments EnvironmentLookup
	teams        TeamLookup
	stats        StatsReader
	authz        Authorizer
	publisher    Publisher
	requests     Requests
}

func NewQueuesUsecase(store Store, services ServiceLookup, environments EnvironmentLookup, teams TeamLookup, stats StatsReader, authz Authorizer, publisher Publisher, requests Requests) QueuesUsecase {
	return QueuesUsecase{
		store: store, services: services, environments: environments, teams: teams,
		stats: stats, authz: authz, publisher: publisher, requests: requests,
	}
}

func (u QueuesUsecase) Get(ctx context.Context, environment, name string) (entities.Queue, error) {
	return u.store.Get(ctx, environment, name)
}

func (u QueuesUsecase) List(ctx context.Context, environment, teamSlug string) ([]entities.Queue, error) {
	if teamSlug == "" {
		return u.store.List(ctx, environment, "")
	}
	teamID, err := u.teams.FindTeam(ctx, teamSlug)
	if err != nil {
		return nil, err
	}
	return u.store.List(ctx, environment, teamID)
}

func (u QueuesUsecase) Versions(ctx context.Context, environment, name string) ([]entities.Version, error) {
	q, err := u.store.Get(ctx, environment, name)
	if err != nil {
		return nil, err
	}
	return u.store.Versions(ctx, q.ID, 0, 50)
}

func (u QueuesUsecase) Stats(ctx context.Context, environment, name string) (entities.Stats, bool, error) {
	q, err := u.store.Get(ctx, environment, name)
	if err != nil {
		return entities.Stats{}, false, err
	}
	st, ok := u.stats.Latest(q.ID)
	return st, ok, nil
}

func (u QueuesUsecase) editable(ctx context.Context, a actor.Actor, environment, name string) (entities.Queue, authz.Gate, error) {
	q, err := u.store.Get(ctx, environment, name)
	if err != nil {
		return entities.Queue{}, "", err
	}
	gate, err := u.authz.Gate(ctx, a, authz.EditQueue, resourceOf(q))
	return q, gate, err
}

func (u QueuesUsecase) operable(ctx context.Context, a actor.Actor, environment, name string) (entities.Queue, authz.Gate, error) {
	q, gate, err := u.editable(ctx, a, environment, name)
	gate, err = authz.Direct(gate, err)
	return q, gate, err
}

func resourceOf(q entities.Queue) authz.Resource {
	return authz.Resource{TeamID: q.TeamID, Environment: q.Environment}
}

func (u QueuesUsecase) publish(ctx context.Context, a actor.Actor, gate authz.Gate, action string, q entities.Queue, details map[string]any) {
	u.publisher.Publish(ctx, events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action, TargetKind: events.TargetQueue,
		TargetID: q.ID, TargetName: q.Name, TeamID: q.TeamID, Environment: q.Environment, Details: events.WithApproval(a, details),
	})
	authz.PublishBypass(ctx, u.publisher, gate, a, authz.EditQueue, resourceOf(q))
}
