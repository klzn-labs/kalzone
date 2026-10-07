package deployments

import (
	"context"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/events"
)

type Store interface {
	Create(ctx context.Context, d entities.Deployment) (entities.Deployment, error)
	Transition(ctx context.Context, d entities.Deployment, from entities.Status) error
	FindByID(ctx context.Context, id string) (entities.Deployment, error)
	History(ctx context.Context, serviceID, environment string, limit int) ([]entities.Deployment, error)
	Active(ctx context.Context, serviceID, environment string) ([]entities.Deployment, error)
	LatestSucceeded(ctx context.Context, serviceID, environment string) (entities.Deployment, bool, error)
	Events(ctx context.Context, deploymentID string, limit int) ([]entities.Event, error)
}

type ServiceLookup interface {
	FindService(ctx context.Context, slug string) (entities.ServiceRef, error)
	FindServiceByID(ctx context.Context, id string) (entities.ServiceRef, error)
}

type ReleaseLookup interface {
	FindRelease(ctx context.Context, id string) (entities.ReleaseRef, error)
}

type ConfigSource interface {
	CurrentNumber(ctx context.Context, serviceSlug, environment string) (int, error)
}

type EnvironmentLookup interface {
	EnvironmentExists(ctx context.Context, slug string) error
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

type DeploymentsUsecase struct {
	store        Store
	services     ServiceLookup
	releases     ReleaseLookup
	configs      ConfigSource
	environments EnvironmentLookup
	authz        Authorizer
	publisher    Publisher
}

func NewDeploymentsUsecase(store Store, services ServiceLookup, releases ReleaseLookup, configs ConfigSource, environments EnvironmentLookup, authz Authorizer, publisher Publisher) DeploymentsUsecase {
	return DeploymentsUsecase{
		store: store, services: services, releases: releases, configs: configs,
		environments: environments, authz: authz, publisher: publisher,
	}
}

func (u DeploymentsUsecase) History(ctx context.Context, serviceSlug, environment string, limit int) ([]entities.Deployment, error) {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	return u.store.History(ctx, service.ID, environment, limit)
}

func (u DeploymentsUsecase) Live(ctx context.Context, serviceSlug, environment string) (entities.Deployment, bool, error) {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return entities.Deployment{}, false, err
	}
	return u.store.LatestSucceeded(ctx, service.ID, environment)
}

func (u DeploymentsUsecase) Get(ctx context.Context, id string) (entities.Deployment, []entities.Event, error) {
	d, err := u.store.FindByID(ctx, id)
	if err != nil {
		return entities.Deployment{}, nil, err
	}
	logged, err := u.store.Events(ctx, id, 500)
	return d, logged, err
}

func publish(ctx context.Context, p Publisher, a actor.Actor, action string, service entities.ServiceRef, d entities.Deployment) {
	p.Publish(ctx, events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, Action: action, TargetKind: events.TargetDeployment,
		TargetID: service.ID, TargetName: service.Slug, TeamID: service.OwnerTeamID, Environment: d.Environment,
		Details: map[string]any{"deployment": d.Number, "release": d.ReleaseNumber, "config": d.ConfigRevision, "trigger": string(d.Trigger)},
	})
}
