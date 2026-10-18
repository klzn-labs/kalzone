package services

import (
	"context"
	"slices"

	"github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var errNotVisible = apperror.NotFound("service not found").WithHint("list services with GET /api/v1/services")

func (u ServicesUsecase) GetFor(ctx context.Context, a actor.Actor, slug string) (entities.Service, error) {
	s, err := u.store.FindBySlug(ctx, slug)
	if err != nil {
		return entities.Service{}, err
	}
	if !a.Sees(s.OwnerTeamID) {
		return entities.Service{}, errNotVisible
	}
	return s, nil
}

func (u ServicesUsecase) ListFor(ctx context.Context, a actor.Actor, teamSlug string) ([]entities.Service, error) {
	list, err := u.listBy(ctx, teamSlug)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list, func(s entities.Service) bool { return !a.Sees(s.OwnerTeamID) }), nil
}

func (u ServicesUsecase) listBy(ctx context.Context, teamSlug string) ([]entities.Service, error) {
	if teamSlug == "" {
		return u.store.List(ctx, entities.ServiceFilter{})
	}
	return u.ListOwnedBy(ctx, teamSlug)
}
