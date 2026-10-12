package queues

import (
	"context"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/events"
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
	Versions(ctx context.Context, queueID string, limit int) ([]entities.Version, error)
	AddCommand(ctx context.Context, c entities.Command) error
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
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
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
}

func NewQueuesUsecase(store Store, services ServiceLookup, environments EnvironmentLookup, teams TeamLookup, stats StatsReader, authz Authorizer, publisher Publisher) QueuesUsecase {
	return QueuesUsecase{
		store: store, services: services, environments: environments, teams: teams,
		stats: stats, authz: authz, publisher: publisher,
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
	return u.store.Versions(ctx, q.ID, 50)
}

func (u QueuesUsecase) Stats(ctx context.Context, environment, name string) (entities.Stats, bool, error) {
	q, err := u.store.Get(ctx, environment, name)
	if err != nil {
		return entities.Stats{}, false, err
	}
	st, ok := u.stats.Latest(q.ID)
	return st, ok, nil
}

func (u QueuesUsecase) editable(ctx context.Context, a actor.Actor, environment, name string) (entities.Queue, error) {
	q, err := u.store.Get(ctx, environment, name)
	if err != nil {
		return entities.Queue{}, err
	}
	return q, u.authz.Can(ctx, a, authz.EditQueue, authz.Resource{TeamID: q.TeamID, Environment: q.Environment})
}

func (u QueuesUsecase) publish(ctx context.Context, a actor.Actor, action string, q entities.Queue, details map[string]any) {
	u.publisher.Publish(ctx, events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action, TargetKind: events.TargetQueue,
		TargetID: q.ID, TargetName: q.Name, TeamID: q.TeamID, Environment: q.Environment, Details: details,
	})
}
