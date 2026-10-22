package catalog

import (
	"context"

	identity "github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/observe/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
)

type TeamLister interface {
	List(ctx context.Context) ([]team.Team, error)
}

type TeamMembers interface {
	UserIDsIn(ctx context.Context, teamIDs []string) ([]string, error)
}

type UserLister interface {
	List(ctx context.Context) ([]identity.User, error)
}

type Roster struct {
	teams   TeamLister
	members TeamMembers
	users   UserLister
}

func NewRoster(teams TeamLister, members TeamMembers, users UserLister) Roster {
	return Roster{teams: teams, members: members, users: users}
}

func (r Roster) Roster(ctx context.Context) ([]entities.OrgTeam, []entities.Person, error) {
	teams, err := r.teams.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	users, err := r.users.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	people := peopleByID(users)
	orgTeams, err := r.orgTeams(ctx, teams, people)
	return orgTeams, peopleOf(people), err
}

func (r Roster) orgTeams(ctx context.Context, teams []team.Team, people map[string]*entities.Person) ([]entities.OrgTeam, error) {
	result := make([]entities.OrgTeam, 0, len(teams))
	for _, t := range teams {
		ids, err := r.members.UserIDsIn(ctx, []string{t.ID})
		if err != nil {
			return nil, err
		}
		result = append(result, entities.OrgTeam{ID: t.ID, Slug: t.Slug, Demo: t.Demo, Members: loginsOf(ids, people, !t.Demo)})
	}
	return result, nil
}

func loginsOf(ids []string, people map[string]*entities.Person, real bool) []string {
	logins := []string{}
	for _, id := range ids {
		p, ok := people[id]
		if !ok {
			continue
		}
		p.OnTeam = p.OnTeam || real
		logins = append(logins, p.Login)
	}
	return logins
}

func peopleByID(users []identity.User) map[string]*entities.Person {
	people := map[string]*entities.Person{}
	for _, u := range users {
		if u.Kind == identity.KindPersona {
			continue
		}
		people[u.ID] = &entities.Person{Login: u.Login, Name: u.Name, Admin: u.Admin}
	}
	return people
}

func peopleOf(byID map[string]*entities.Person) []entities.Person {
	people := make([]entities.Person, 0, len(byID))
	for _, p := range byID {
		people = append(people, *p)
	}
	return people
}
