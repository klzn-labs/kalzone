package web

import (
	"context"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
)

type ServiceCard struct {
	Service catalog.Service
	Team    team.Team
	Envs    []EnvChip
}

type EnvChip struct {
	Slug    string
	Release int
}

type MyTeam struct {
	Team team.Team
	Role team.Role
}

func (v Views) ServiceCards(ctx context.Context, services []catalog.Service) ([]ServiceCard, error) {
	teams, err := v.TeamsByID(ctx, ownerIDs(services))
	if err != nil {
		return nil, err
	}
	envs, err := v.environments.List(ctx)
	if err != nil {
		return nil, err
	}
	cards := make([]ServiceCard, 0, len(services))
	for _, s := range services {
		chips, err := v.envChips(ctx, s.Slug, envs)
		if err != nil {
			return nil, err
		}
		cards = append(cards, ServiceCard{Service: s, Team: teams[s.OwnerTeamID], Envs: chips})
	}
	return cards, nil
}

func (v Views) envChips(ctx context.Context, serviceSlug string, envs []catalog.Environment) ([]EnvChip, error) {
	chips := make([]EnvChip, 0, len(envs))
	for _, env := range envs {
		live, _, err := v.live.Live(ctx, serviceSlug, env.Slug)
		if err != nil {
			return nil, err
		}
		chips = append(chips, EnvChip{Slug: env.Slug, Release: live.ReleaseNumber})
	}
	return chips, nil
}

func ownerIDs(services []catalog.Service) []string {
	ids := make([]string, 0, len(services))
	for _, s := range services {
		ids = append(ids, s.OwnerTeamID)
	}
	return ids
}

func (v Views) MyTeams(ctx context.Context, userID string) ([]MyTeam, error) {
	memberships, err := v.memberships.MembershipsOf(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(memberships))
	for _, m := range memberships {
		ids = append(ids, m.TeamID)
	}
	teams, err := v.TeamsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]MyTeam, 0, len(memberships))
	for _, m := range memberships {
		result = append(result, MyTeam{Team: teams[m.TeamID], Role: m.Role})
	}
	return result, nil
}

func TeamIDs(teams []MyTeam) []string {
	ids := make([]string, 0, len(teams))
	for _, t := range teams {
		ids = append(ids, t.Team.ID)
	}
	return ids
}
