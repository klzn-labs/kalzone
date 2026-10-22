package fakestore

import (
	"context"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var (
	API     = entities.ServiceRef{ID: "s1", Slug: "api", OwnerTeamID: "t1"}
	Worker  = entities.ServiceRef{ID: "s2", Slug: "worker", OwnerTeamID: "t1"}
	Retired = entities.ServiceRef{ID: "s3", Slug: "retired", OwnerTeamID: "t1", Decommissioning: true}
	Demo    = entities.ServiceRef{ID: "s4", Slug: "demo-shop", OwnerTeamID: "t1", Simulated: true}
)

type Services struct{}

func (Services) FindService(_ context.Context, slug string) (entities.ServiceRef, error) {
	return find(func(s entities.ServiceRef) bool { return s.Slug == slug })
}

func (Services) FindServiceByID(_ context.Context, id string) (entities.ServiceRef, error) {
	return find(func(s entities.ServiceRef) bool { return s.ID == id })
}

func find(match func(entities.ServiceRef) bool) (entities.ServiceRef, error) {
	for _, s := range []entities.ServiceRef{API, Worker, Retired, Demo} {
		if match(s) {
			return s, nil
		}
	}
	return entities.ServiceRef{}, apperror.NotFound("service not found").WithHint("list services with GET /api/v1/services")
}
