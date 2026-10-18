package query

import (
	"context"

	"github.com/klzn-labs/kalzone/app/audit/entities"
)

const (
	defaultLimit = 50
	maxLimit     = 200
)

type EventStore interface {
	List(ctx context.Context, filter entities.Filter) ([]entities.Event, error)
}

type QueryUsecase struct {
	store EventStore
}

func NewQueryUsecase(store EventStore) QueryUsecase {
	return QueryUsecase{store: store}
}

func (u QueryUsecase) List(ctx context.Context, filter entities.Filter) ([]entities.Event, error) {
	switch {
	case filter.Limit <= 0:
		filter.Limit = defaultLimit
	case filter.Limit > maxLimit:
		filter.Limit = maxLimit
	}
	return u.store.List(ctx, filter)
}
