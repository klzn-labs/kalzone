package orgs_test

import (
	"context"
	"testing"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/observe/usecases/orgs"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type grafana struct {
	enabled    bool
	orgs       map[string]int
	members    map[int]map[string]string
	users      map[string]bool
	sources    map[int][]string
	dashboards map[int]map[string]bool
	nextID     int
}

func newGrafana() *grafana {
	return &grafana{
		enabled: true, orgs: map[string]int{"Main Org.": 1}, members: map[int]map[string]string{1: {"admin": "Admin"}},
		users: map[string]bool{}, sources: map[int][]string{}, dashboards: map[int]map[string]bool{}, nextID: 2,
	}
}

func (g *grafana) Enabled() bool      { return g.enabled }
func (g *grafana) AdminLogin() string { return "admin" }

func (g *grafana) Orgs(context.Context) ([]entities.Org, error) {
	all := []entities.Org{}
	for name, id := range g.orgs {
		all = append(all, entities.Org{ID: id, Name: name})
	}
	return all, nil
}

func (g *grafana) CreateOrg(_ context.Context, name string) (int, error) {
	id := g.nextID
	g.nextID++
	g.orgs[name], g.members[id], g.dashboards[id] = id, map[string]string{"admin": "Admin"}, map[string]bool{}
	return id, nil
}

func (g *grafana) DeleteOrg(_ context.Context, id int) error {
	for name, orgID := range g.orgs {
		if orgID == id {
			delete(g.orgs, name)
		}
	}
	return nil
}

func (g *grafana) EnsureUser(_ context.Context, p entities.Person) error {
	g.users[p.Login] = true
	return nil
}

func (g *grafana) Members(_ context.Context, org int) ([]entities.OrgMember, error) {
	members := []entities.OrgMember{}
	for login, role := range g.members[org] {
		members = append(members, entities.OrgMember{UserID: len(login), Login: login, Role: role})
	}
	return members, nil
}

func (g *grafana) AddMember(_ context.Context, org int, login, role string) error {
	g.members[org][login] = role
	return nil
}

func (g *grafana) SetRole(_ context.Context, org, userID int, role string) error {
	for login := range g.members[org] {
		if len(login) == userID {
			g.members[org][login] = role
		}
	}
	return nil
}

func (g *grafana) RemoveMember(_ context.Context, org, userID int) error {
	for login := range g.members[org] {
		if len(login) == userID {
			delete(g.members[org], login)
		}
	}
	return nil
}

func (g *grafana) UpsertDatasource(_ context.Context, org int, d entities.Datasource) error {
	g.sources[org] = append(g.sources[org], d.UID+" "+d.URL)
	return nil
}

func (g *grafana) EnsureFolder(context.Context, int) error { return nil }

func (g *grafana) UpsertDashboard(_ context.Context, org int, d map[string]any) error {
	g.dashboards[org][d["uid"].(string)] = true
	return nil
}

func (g *grafana) DeleteDashboard(_ context.Context, org int, uid string) error {
	delete(g.dashboards[org], uid)
	return nil
}

func (g *grafana) DashboardUIDs(_ context.Context, org int) ([]string, error) {
	uids := []string{}
	for uid := range g.dashboards[org] {
		uids = append(uids, uid)
	}
	return uids, nil
}

type roster struct {
	teams  []entities.OrgTeam
	people []entities.Person
}

func (r *roster) Roster(context.Context) ([]entities.OrgTeam, []entities.Person, error) {
	return r.teams, r.people, nil
}

type simulated []entities.Service

func (s simulated) Simulated(context.Context) ([]entities.Service, error) { return s, nil }

type tokens struct{ enabled bool }

func (t tokens) Enabled() bool            { return t.enabled }
func (t tokens) Token(team string) string { return "tok-" + team }

func places() *roster {
	return &roster{
		teams:  []entities.OrgTeam{{ID: "t1", Slug: "places", Members: []string{"bob"}}, {ID: "d1", Slug: "demo-shop", Demo: true}},
		people: []entities.Person{{Login: "bob", OnTeam: true}, {Login: "root", Admin: true}, {Login: "guest.demo"}},
	}
}

func setup(g *grafana, r *roster) orgs.OrgsUsecase {
	return orgs.NewOrgsUsecase(g, r, simulated{{Slug: "demo-shop-api", TeamID: "d1"}}, tokens{enabled: true}, "http://controlplane:8080/")
}

var placesAPI = []entities.Service{{Slug: "places-api", TeamID: "t1"}}

func Test_Sync_CreatesAnOrgPerTeamWithItsPeopleDatasourcesAndDashboards(t *testing.T) {
	g := newGrafana()
	uc := setup(g, places())

	require.NoError(t, uc.Sync(ctx, placesAPI))

	id, ok := uc.OrgID("team-places")
	require.True(t, ok)
	require.Equal(t, map[string]string{"admin": "Admin", "root": "Admin", "bob": "Viewer"}, g.members[id])
	require.Contains(t, g.sources[id], "prometheus http://controlplane:8080/telemetry/places/prometheus")
	require.Equal(t, map[string]bool{"platform-places-api": true}, g.dashboards[id])
	require.True(t, g.users["guest.demo"])
	demo, _ := uc.OrgID("demo")
	require.Equal(t, "Viewer", g.members[demo]["guest.demo"])
	require.Equal(t, map[string]bool{"platform-demo-shop-api": true}, g.dashboards[demo])
}

func Test_Sync_FollowsMembershipRoleAndOwnershipChanges(t *testing.T) {
	g, r := newGrafana(), places()
	uc := setup(g, r)
	require.NoError(t, uc.Sync(ctx, placesAPI))
	r.teams = append(r.teams, entities.OrgTeam{ID: "t2", Slug: "maps", Members: []string{"bob"}})
	r.teams[0].Members = nil
	r.people[1].Admin = false

	require.NoError(t, uc.Sync(ctx, []entities.Service{{Slug: "places-api", TeamID: "t2"}}))

	placesOrg, _ := uc.OrgID("team-places")
	mapsOrg, _ := uc.OrgID("team-maps")
	require.Equal(t, map[string]string{"admin": "Admin"}, g.members[placesOrg])
	require.Empty(t, g.dashboards[placesOrg])
	require.Equal(t, map[string]bool{"platform-places-api": true}, g.dashboards[mapsOrg])
	require.Equal(t, "Viewer", g.members[mapsOrg]["bob"])
}

func Test_Sync_DeletesTheOrgsOfTeamsThatAreGoneButNeverOthers(t *testing.T) {
	g, r := newGrafana(), places()
	g.orgs["Someone's org"] = 50
	uc := setup(g, r)
	require.NoError(t, uc.Sync(ctx, placesAPI))
	r.teams = nil

	require.NoError(t, uc.Sync(ctx, nil))

	_, places := g.orgs["team-places"]
	_, demo := g.orgs["demo"]
	_, theirs := g.orgs["Someone's org"]
	_, known := uc.OrgID("team-places")
	require.False(t, places)
	require.False(t, demo)
	require.True(t, theirs)
	require.False(t, known)
}

func Test_Sync_DoesNothingWithoutGrafanaOrTelemetry(t *testing.T) {
	off := newGrafana()
	off.enabled = false
	noTokens := newGrafana()

	require.NoError(t, setup(off, places()).Sync(ctx, placesAPI))
	require.NoError(t, orgs.NewOrgsUsecase(noTokens, places(), simulated{}, tokens{}, "").Sync(ctx, placesAPI))

	require.Len(t, off.orgs, 1)
	require.Len(t, noTokens.orgs, 1)
}
