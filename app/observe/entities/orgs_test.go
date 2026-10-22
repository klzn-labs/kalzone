package entities_test

import (
	"encoding/json"
	"testing"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/stretchr/testify/require"
)

var orgInputs = entities.OrgInputs{
	Teams: []entities.OrgTeam{
		{ID: "t1", Slug: "places", Members: []string{"bob", "root"}},
		{ID: "t2", Slug: "empty"},
		{ID: "d1", Slug: "demo-shop", Demo: true, Members: []string{"persona"}},
	},
	People: []entities.Person{
		{Login: "root", Admin: true, OnTeam: true}, {Login: "bob", OnTeam: true},
		{Login: "guest.demo"}, {Login: "newcomer"},
	},
	Services:  []entities.Service{{Slug: "places-api", Name: "Places API", TeamID: "t1"}},
	Simulated: []entities.Service{{Slug: "demo-shop-api", Name: "Shop API"}},
	Telemetry: entities.TelemetryLink{URL: "http://controlplane:8080", Tokens: map[string]string{"places": "tok"}},
}

func planNamed(t *testing.T, name string) entities.OrgPlan {
	t.Helper()
	for _, p := range entities.OrgPlans(orgInputs) {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no plan for %s", name)
	return entities.OrgPlan{}
}

func Test_OrgPlans_GiveEveryTeamAnOrgAndTheDemoTeamsOneDemoOrg(t *testing.T) {
	names := []string{}
	for _, p := range entities.OrgPlans(orgInputs) {
		names = append(names, p.Name)
	}

	require.Equal(t, []string{"team-places", "team-empty", "demo"}, names)
}

func Test_ATeamOrgHoldsItsMembersAsViewersAndEveryAdmin(t *testing.T) {
	require.Equal(t, map[string]string{"root": "Admin", "bob": "Viewer"}, planNamed(t, "team-places").Members)
	require.Equal(t, map[string]string{"root": "Admin"}, planNamed(t, "team-empty").Members)
}

func Test_ATeamOrgReadsThroughItsOwnTelemetryEndpoint(t *testing.T) {
	sources := planNamed(t, "team-places").Datasources

	require.Equal(t, "http://controlplane:8080/telemetry/places/prometheus", sources[0].URL)
	require.Equal(t, "prometheus", sources[0].UID)
	require.Equal(t, "loki", sources[1].UID)
	require.Equal(t, "Bearer tok", sources[1].Secure["httpHeaderValue1"])
	require.Equal(t, "Authorization", sources[1].JSONData["httpHeaderName1"])
}

func Test_ATeamOrgHoldsOnlyItsServicesDashboards(t *testing.T) {
	places := planNamed(t, "team-places").Dashboards

	require.Len(t, places, 1)
	require.Equal(t, "platform-places-api", places[0]["uid"])
	require.Empty(t, planNamed(t, "team-empty").Dashboards)
}

func Test_TheDemoOrgHoldsTheGuestAndPeopleOnNoTeam(t *testing.T) {
	demo := planNamed(t, "demo")

	require.Equal(t, map[string]string{"root": "Admin", "guest.demo": "Viewer", "newcomer": "Viewer"}, demo.Members)
	require.Equal(t, entities.TestDataUID, demo.Datasources[0].UID)
	require.Equal(t, "platform-demo-shop-api", demo.Dashboards[0]["uid"])
}

func Test_DemoDashboard_ReadsOnlyTestData(t *testing.T) {
	raw, err := json.Marshal(entities.DemoDashboard(entities.Service{Slug: "demo-shop-api", Name: "Shop API"}))
	require.NoError(t, err)
	body := string(raw)

	require.NotContains(t, body, `"uid":"prometheus"`)
	require.NotContains(t, body, `"uid":"loki"`)
	require.Contains(t, body, `"scenarioId":"random_walk"`)
	require.Contains(t, body, `"scenarioId":"logs"`)
}
