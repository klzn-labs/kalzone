package runtime

import (
	"context"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
)

var (
	errServiceRequired     = apperror.Invalid("service is required").WithField("service").WithHint("list services with GET /api/v1/services")
	errEnvironmentRequired = apperror.Invalid("env is required").WithField("env").WithHint("list environments with GET /api/v1/environments")
	errServiceNotVisible   = apperror.NotFound("service not found").WithHint("list services with GET /api/v1/services")
	errNotReported         = apperror.NotFound("no runtime status yet").WithHint("the agent reports every 15s; deploy the service first")
)

type Store interface {
	Runtime(ctx context.Context, serviceID, environment string) (entities.RuntimeStatus, bool, error)
}

type ServiceLookup interface {
	FindService(ctx context.Context, slug string) (entities.ServiceRef, error)
}

type Meter interface {
	Usage(ctx context.Context, container string) (cpuMillis, memoryMB int, ok bool)
}

type RuntimeUsecase struct {
	store    Store
	services ServiceLookup
	meter    Meter
}

func NewRuntimeUsecase(store Store, services ServiceLookup, meter Meter) RuntimeUsecase {
	return RuntimeUsecase{store: store, services: services, meter: meter}
}

func (u RuntimeUsecase) StatusFor(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.RuntimeView, error) {
	service, err := u.visibleService(ctx, a, serviceSlug, environment)
	if err != nil {
		return entities.RuntimeView{}, err
	}
	status, found, err := u.store.Runtime(ctx, service.ID, environment)
	if err != nil || !found {
		return entities.RuntimeView{}, firstOr(err, errNotReported)
	}
	view := entities.RuntimeView{Service: service.Slug, Environment: environment, Status: status, Stale: status.Stale(clock.Now())}
	view.Usage = u.usage(ctx, status)
	return view, nil
}

func (u RuntimeUsecase) visibleService(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.ServiceRef, error) {
	switch {
	case serviceSlug == "":
		return entities.ServiceRef{}, errServiceRequired
	case environment == "":
		return entities.ServiceRef{}, errEnvironmentRequired
	}
	service, err := u.services.FindService(ctx, serviceSlug)
	if err == nil && !a.Sees(service.OwnerTeamID) {
		return entities.ServiceRef{}, errServiceNotVisible
	}
	return service, err
}

func (u RuntimeUsecase) usage(ctx context.Context, status entities.RuntimeStatus) *entities.Usage {
	if !status.Found || status.Container == "" {
		return nil
	}
	cpu, memory, ok := u.meter.Usage(ctx, status.Container)
	if !ok {
		return nil
	}
	return &entities.Usage{CPUMillis: cpu, CPULimitMillis: status.CPULimitMillis, MemoryMB: memory, MemoryLimitMB: status.MemoryLimitMB}
}

func firstOr(err, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}
