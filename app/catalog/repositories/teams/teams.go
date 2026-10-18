package teams

import (
	"context"

	team "github.com/klzn-labs/kalzone/app/team/entities"
)

type TeamFinder interface {
	Get(ctx context.Context, slug string) (team.Team, error)
}

type Lookup struct {
	teams TeamFinder
}

func NewLookup(teams TeamFinder) Lookup {
	return Lookup{teams: teams}
}

func (l Lookup) IDBySlug(ctx context.Context, slug string) (string, error) {
	t, err := l.teams.Get(ctx, slug)
	return t.ID, err
}
