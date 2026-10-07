package environments

import (
	"context"

	"github.com/adrianozp/platform/app/catalog/entities"
)

type EnvironmentStore interface {
	List(ctx context.Context) ([]entities.Environment, error)
	Get(ctx context.Context, slug string) (entities.Environment, error)
}

type EnvironmentsUsecase struct {
	store EnvironmentStore
}

func NewEnvironmentsUsecase(store EnvironmentStore) EnvironmentsUsecase {
	return EnvironmentsUsecase{store: store}
}

func (u EnvironmentsUsecase) List(ctx context.Context) ([]entities.Environment, error) {
	return u.store.List(ctx)
}

func (u EnvironmentsUsecase) Get(ctx context.Context, slug string) (entities.Environment, error) {
	return u.store.Get(ctx, slug)
}

func (u EnvironmentsUsecase) IsProtected(ctx context.Context, slug string) (bool, error) {
	env, err := u.store.Get(ctx, slug)
	return env.Protected, err
}
