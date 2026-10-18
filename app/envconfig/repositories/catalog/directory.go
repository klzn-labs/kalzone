package catalog

import (
	"context"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/envconfig/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
)

type ServiceLister interface {
	List(ctx context.Context, filter catalog.ServiceFilter) ([]catalog.Service, error)
}

type TeamFinder interface {
	Get(ctx context.Context, teamSlug string) (team.Team, error)
}

type Directory struct {
	services ServiceLister
	teams    TeamFinder
}

func NewDirectory(services ServiceLister, teams TeamFinder) Directory {
	return Directory{services: services, teams: teams}
}

func (d Directory) Services(ctx context.Context) ([]entities.ServiceRef, error) {
	list, err := d.services.List(ctx, catalog.ServiceFilter{})
	if err != nil {
		return nil, err
	}
	refs := make([]entities.ServiceRef, 0, len(list))
	for _, s := range list {
		refs = append(refs, entities.ServiceRef{ID: s.ID, Slug: s.Slug, OwnerTeamID: s.OwnerTeamID})
	}
	return refs, nil
}

func (d Directory) TeamID(ctx context.Context, teamSlug string) (string, error) {
	t, err := d.teams.Get(ctx, teamSlug)
	return t.ID, err
}
