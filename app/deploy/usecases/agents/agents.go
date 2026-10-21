package agents

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/events"
)

const (
	claimAttempts  = 3
	maxLines       = 200
	maxLineLength  = 1000
	eventsPerQuery = 500
)

type Store interface {
	Create(ctx context.Context, d entities.Deployment) (entities.Deployment, error)
	Transition(ctx context.Context, d entities.Deployment, from entities.Status) error
	FindByID(ctx context.Context, id string) (entities.Deployment, error)
	Active(ctx context.Context, serviceID, environment string) ([]entities.Deployment, error)
	LatestSucceeded(ctx context.Context, serviceID, environment string) (entities.Deployment, bool, error)
	AllLive(ctx context.Context) ([]entities.Deployment, error)
	OldestPending(ctx context.Context) (entities.Deployment, bool, error)
	StaleInProgress(ctx context.Context, startedBefore time.Time) ([]entities.Deployment, error)
	AppendEvents(ctx context.Context, deploymentID string, at time.Time, lines []string) error
	TouchAgent(ctx context.Context, a entities.Agent) error
	SaveRuntime(ctx context.Context, statuses []entities.RuntimeStatus) error
	SavePlatform(ctx context.Context, c entities.PlatformComponent) error
	Agents(ctx context.Context) ([]entities.Agent, error)
}

type ServiceLookup interface {
	FindService(ctx context.Context, slug string) (entities.ServiceRef, error)
	FindServiceByID(ctx context.Context, id string) (entities.ServiceRef, error)
}

type ConfigResolver interface {
	ResolveConfig(ctx context.Context, serviceID, environment string, number int) (entities.ResolvedConfig, error)
}

type QueueAccess interface {
	QueueAccess(ctx context.Context, serviceID, environment string) (env, secrets map[string]string, err error)
}

type DatabaseAccess interface {
	DatabaseAccess(ctx context.Context, serviceID, environment string) (env, secrets map[string]string, err error)
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

type AgentsUsecase struct {
	store     Store
	services  ServiceLookup
	configs   ConfigResolver
	queues    QueueAccess
	databases DatabaseAccess
	publisher Publisher
	timeout   time.Duration
}

func NewAgentsUsecase(cfg config.Config, store Store, services ServiceLookup, configs ConfigResolver, queues QueueAccess, databases DatabaseAccess, publisher Publisher) AgentsUsecase {
	return AgentsUsecase{
		store: store, services: services, configs: configs, queues: queues, databases: databases, publisher: publisher, timeout: cfg.Deploy.Timeout,
	}
}

func (u AgentsUsecase) Agents(ctx context.Context) ([]entities.Agent, error) {
	return u.store.Agents(ctx)
}

func agentActor(name string) actor.Actor {
	return actor.Actor{Login: "agent:" + name}
}

func (u AgentsUsecase) publish(ctx context.Context, a actor.Actor, action string, d entities.Deployment, detail string) {
	service, err := u.services.FindServiceByID(ctx, d.ServiceID)
	if err != nil {
		return
	}
	details := map[string]any{"deployment": d.Number, "release": d.ReleaseNumber, "config": d.ConfigRevision, "trigger": string(d.Trigger)}
	if detail != "" {
		details["message"] = detail
	}
	u.publisher.Publish(ctx, events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action, TargetKind: events.TargetDeployment,
		TargetID: service.ID, TargetName: service.Slug, TeamID: service.OwnerTeamID, Environment: d.Environment, Details: details,
	})
}
