package decommission

import (
	"context"
	"log"
	"maps"
	"slices"

	"github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/events"
)

var Actor = actor.Actor{Login: "decommission"}

type Services interface {
	MarkDecommissioning(ctx context.Context, a actor.Actor, slug string) (entities.Service, error)
	MarkDeleted(ctx context.Context, a actor.Actor, id string) error
	GetByID(ctx context.Context, id string) (entities.Service, error)
}

type Host interface {
	Remove(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.TeardownStep, error)
	Freeze(ctx context.Context, serviceID string) error
	Teardown(ctx context.Context, serviceID string) (map[string]bool, error)
	Removing(ctx context.Context, serviceID, environment string) (bool, error)
	ForgetRuntime(ctx context.Context, serviceID string) error
}

type Releases interface {
	FailUnfinished(ctx context.Context, a actor.Actor, serviceID string) error
}

type Requests interface {
	CancelForService(ctx context.Context, a actor.Actor, serviceID string) error
}

type Queues interface {
	Detach(ctx context.Context, a actor.Actor, serviceID string) error
}

type Dashboards interface {
	Delete(ctx context.Context, serviceSlug string) error
}

type DecommissionUsecase struct {
	services   Services
	host       Host
	releases   Releases
	requests   Requests
	queues     Queues
	dashboards Dashboards
}

func NewDecommissionUsecase(services Services, host Host, releases Releases, requests Requests, queues Queues, dashboards Dashboards) DecommissionUsecase {
	return DecommissionUsecase{services: services, host: host, releases: releases, requests: requests, queues: queues, dashboards: dashboards}
}

func (u DecommissionUsecase) Start(ctx context.Context, a actor.Actor, slug string) (entities.Decommission, error) {
	service, err := u.services.MarkDecommissioning(ctx, a, slug)
	if err != nil {
		return entities.Decommission{}, err
	}
	if err := u.quiesce(ctx, a, service.ID); err != nil {
		return entities.Decommission{}, err
	}
	steps, err := u.tearDown(ctx, a, service)
	if err != nil {
		return entities.Decommission{}, err
	}
	deleted, err := u.Finish(ctx, service.ID)
	return entities.Decommission{Service: service, Steps: steps, Deleted: deleted}, err
}

func (u DecommissionUsecase) quiesce(ctx context.Context, a actor.Actor, serviceID string) error {
	if err := u.host.Freeze(ctx, serviceID); err != nil {
		return err
	}
	if err := u.requests.CancelForService(ctx, a, serviceID); err != nil {
		return err
	}
	return u.releases.FailUnfinished(ctx, a, serviceID)
}

func (u DecommissionUsecase) tearDown(ctx context.Context, a actor.Actor, service entities.Service) ([]entities.TeardownStep, error) {
	removed, err := u.host.Teardown(ctx, service.ID)
	if err != nil {
		return nil, err
	}
	steps := []entities.TeardownStep{}
	for _, env := range slices.Sorted(maps.Keys(removed)) {
		step, err := u.tearDownOne(ctx, a, service, env, removed[env])
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func (u DecommissionUsecase) tearDownOne(ctx context.Context, a actor.Actor, service entities.Service, env string, removed bool) (entities.TeardownStep, error) {
	if removed {
		return entities.TeardownStep{Environment: env, Status: entities.TeardownRemoved}, nil
	}
	removing, err := u.host.Removing(ctx, service.ID, env)
	if err != nil || removing {
		return entities.TeardownStep{Environment: env, Status: entities.TeardownRemoving}, err
	}
	return u.host.Remove(ctx, a, service.Slug, env)
}

func (u DecommissionUsecase) Finish(ctx context.Context, serviceID string) (bool, error) {
	service, ready, err := u.readyToFinish(ctx, serviceID)
	if err != nil || !ready {
		return false, err
	}
	if err := u.queues.Detach(ctx, Actor, serviceID); err != nil {
		return false, err
	}
	if err := u.host.ForgetRuntime(ctx, serviceID); err != nil {
		return false, err
	}
	if err := u.dashboards.Delete(ctx, service.Slug); err != nil {
		log.Printf("dashboard of %s left in Grafana: %v", service.Slug, err)
	}
	return true, u.services.MarkDeleted(ctx, Actor, serviceID)
}

func (u DecommissionUsecase) readyToFinish(ctx context.Context, serviceID string) (entities.Service, bool, error) {
	service, err := u.services.GetByID(ctx, serviceID)
	if err != nil || !service.Decommissioning() {
		return service, false, err
	}
	removed, err := u.host.Teardown(ctx, serviceID)
	for _, done := range removed {
		if !done {
			return service, false, err
		}
	}
	return service, err == nil, err
}

func (u DecommissionUsecase) Handle(ctx context.Context, e events.Event) error {
	if e.Action != "deploy.succeeded" || e.Details["trigger"] != "decommission" {
		return nil
	}
	_, err := u.Finish(ctx, e.TargetID)
	return err
}
