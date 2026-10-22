package teams

import (
	"context"

	team "github.com/klzn-labs/kalzone/app/team/entities"
)

type TeamFinder interface {
	Get(ctx context.Context, slug string) (team.Team, error)
	FindByIDs(ctx context.Context, ids []string) ([]team.Team, error)
}

type MemberTeams interface {
	TeamIDsOf(ctx context.Context, userID string) ([]string, error)
	UserIDsIn(ctx context.Context, teamIDs []string) ([]string, error)
}

type Lookup struct {
	teams   TeamFinder
	members MemberTeams
}

func NewLookup(teams TeamFinder, members MemberTeams) Lookup {
	return Lookup{teams: teams, members: members}
}

func (l Lookup) UserIDsIn(ctx context.Context, teamIDs []string) ([]string, error) {
	return l.members.UserIDsIn(ctx, teamIDs)
}

func (l Lookup) VisibleTeamIDs(ctx context.Context, userID string) ([]string, error) {
	return l.members.TeamIDsOf(ctx, userID)
}

func (l Lookup) IDBySlug(ctx context.Context, slug string) (string, error) {
	t, err := l.teams.Get(ctx, slug)
	return t.ID, err
}

func (l Lookup) SlugByID(ctx context.Context, ids []string) (map[string]string, error) {
	found, err := l.teams.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	slugs := make(map[string]string, len(found))
	for _, t := range found {
		slugs[t.ID] = t.Slug
	}
	return slugs, nil
}

func (l Lookup) SlugsByIDs(ctx context.Context, ids []string) ([]string, error) {
	found, err := l.teams.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(found))
	for _, t := range found {
		slugs = append(slugs, t.Slug)
	}
	return slugs, nil
}
