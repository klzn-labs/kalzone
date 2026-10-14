package fakestore

import (
	"context"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/pkg/apperror"
)

var (
	API    = entities.ServiceRef{ID: "s1", Slug: "api", OwnerTeamID: "t1"}
	Worker = entities.ServiceRef{ID: "s2", Slug: "worker", OwnerTeamID: "t1"}
)

type Services struct{}

func (Services) FindService(_ context.Context, slug string) (entities.ServiceRef, error) {
	return find(func(s entities.ServiceRef) bool { return s.Slug == slug })
}

func (Services) FindServiceByID(_ context.Context, id string) (entities.ServiceRef, error) {
	return find(func(s entities.ServiceRef) bool { return s.ID == id })
}

func find(match func(entities.ServiceRef) bool) (entities.ServiceRef, error) {
	for _, s := range []entities.ServiceRef{API, Worker} {
		if match(s) {
			return s, nil
		}
	}
	return entities.ServiceRef{}, apperror.NotFound("service not found").WithHint("list services with GET /api/v1/services")
}
