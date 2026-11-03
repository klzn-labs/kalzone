package catalog_test

import (
	"context"
	"sort"
	"testing"
	"time"

	identity "github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/observe/repositories/catalog"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/stretchr/testify/require"
)

type teamList []team.Team

func (l teamList) List(context.Context) ([]team.Team, error) { return l, nil }

type members map[string][]string

func (m members) UserIDsIn(_ context.Context, ids []string) ([]string, error) { return m[ids[0]], nil }

type userList []identity.User

func (l userList) List(context.Context) ([]identity.User, error) { return l, nil }

func Test_Roster_NamesTeamMembersAndWhoIsOnARealTeam(t *testing.T) {
	teams := teamList{{ID: "t1", Slug: "places"}, {ID: "d1", Slug: "demo-shop", Demo: true}}
	users := userList{
		{ID: "u1", Login: "bob", Kind: identity.KindPerson}, {ID: "u2", Login: "root", Admin: true, Kind: identity.KindPerson},
		{ID: "u3", Login: "guest.demo", Kind: identity.KindGuest}, {ID: "u4", Login: "dana", Kind: identity.KindPersona},
	}

	orgTeams, people, err := catalog.NewRoster(teams, members{"t1": {"u1"}, "d1": {"u4", "u3"}}, users).Roster(context.Background())
	sort.Slice(people, func(i, j int) bool { return people[i].Login < people[j].Login })

	require.NoError(t, err)
	require.Equal(t, []entities.OrgTeam{{ID: "t1", Slug: "places", Members: []string{"bob"}}, {ID: "d1", Slug: "demo-shop", Demo: true, Members: []string{"guest.demo"}}}, orgTeams)
	require.Equal(t, []entities.Person{{Login: "bob", OnTeam: true}, {Login: "guest.demo"}, {Login: "root", Admin: true}}, people)
}

func Test_Roster_LeavesOutRemovedPeople(t *testing.T) {
	removedAt := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	users := userList{{ID: "u1", Login: "bob", Kind: identity.KindPerson}, {ID: "u2", Login: "gone", Kind: identity.KindPerson, DisabledAt: &removedAt}}

	orgTeams, people, err := catalog.NewRoster(teamList{{ID: "t1", Slug: "places"}}, members{"t1": {"u1", "u2"}}, users).Roster(context.Background())

	require.NoError(t, err)
	require.Equal(t, []entities.OrgTeam{{ID: "t1", Slug: "places", Members: []string{"bob"}}}, orgTeams)
	require.Equal(t, []entities.Person{{Login: "bob", OnTeam: true}}, people)
}
