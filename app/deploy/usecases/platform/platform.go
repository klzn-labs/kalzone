package platform

import (
	"context"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var errNotReported = apperror.NotFound("platform component not reported yet").WithHint("the agent reports it every status tick")

type Store interface {
	Platform(ctx context.Context, name string) (entities.PlatformComponent, bool, error)
}

type PlatformUsecase struct {
	store Store
}

func NewPlatformUsecase(store Store) PlatformUsecase {
	return PlatformUsecase{store: store}
}

func (u PlatformUsecase) Component(ctx context.Context, name string) (entities.PlatformComponent, error) {
	if !entities.IsPlatformComponent(name) {
		return entities.PlatformComponent{}, apperror.NotFound("unknown platform component")
	}
	c, found, err := u.store.Platform(ctx, name)
	if err == nil && !found {
		return entities.PlatformComponent{Name: name}, errNotReported
	}
	return c, err
}
