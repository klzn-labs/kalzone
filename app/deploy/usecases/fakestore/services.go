package fakestore

import (
	"context"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/pkg/apperror"
)

var API = entities.ServiceRef{ID: "s1", Slug: "api", OwnerTeamID: "t1"}

type Services struct{}

func (Services) FindService(_ context.Context, slug string) (entities.ServiceRef, error) {
	if slug != API.Slug {
		return entities.ServiceRef{}, apperror.NotFound("service not found")
	}
	return API, nil
}

func (Services) FindServiceByID(_ context.Context, id string) (entities.ServiceRef, error) {
	if id != API.ID {
		return entities.ServiceRef{}, apperror.NotFound("service not found")
	}
	return API, nil
}
