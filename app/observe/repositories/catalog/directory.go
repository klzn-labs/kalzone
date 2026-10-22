package catalog

import (
	"context"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
)

type TeamFinder interface {
	Get(ctx context.Context, teamSlug string) (team.Team, error)
}

type EnvironmentLister interface {
	List(ctx context.Context) ([]catalog.Environment, error)
}

type Directory struct {
	teams        TeamFinder
	environments EnvironmentLister
}

func NewDirectory(teams TeamFinder, environments EnvironmentLister) Directory {
	return Directory{teams: teams, environments: environments}
}

func (d Directory) TeamID(ctx context.Context, teamSlug string) (string, error) {
	t, err := d.teams.Get(ctx, teamSlug)
	return t.ID, err
}

func (d Directory) Environments(ctx context.Context) ([]string, error) {
	all, err := d.environments.List(ctx)
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(all))
	for _, env := range all {
		slugs = append(slugs, env.Slug)
	}
	return slugs, nil
}
