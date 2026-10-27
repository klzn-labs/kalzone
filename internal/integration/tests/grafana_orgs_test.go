package tests

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/stretchr/testify/require"
)

func orgOf(t *testing.T, s *integration.Suite, name string) int {
	t.Helper()
	var id int
	require.Eventually(t, func() bool {
		var ok bool
		id, ok = s.Grafana.OrgID(name)
		return ok
	}, 5*time.Second, 50*time.Millisecond, name)
	return id
}

func Test_EveryTeamGetsAGrafanaOrgWithItsPeopleDashboardsAndFilteredDatasources(t *testing.T) {
	s, _ := setupPlaces(t)

	org := orgOf(t, s, "team-places")

	require.Eventually(t, func() bool { return s.Grafana.Members(org)["bob"] == "Viewer" }, 5*time.Second, 50*time.Millisecond)
	require.Equal(t, map[string]string{"admin": "Admin", "root": "Admin", "mia": "Viewer", "bob": "Viewer"}, s.Grafana.Members(org))
	require.Eventually(t, func() bool { return s.Grafana.OrgDashboards(org)["platform-places-api"] }, 5*time.Second, 50*time.Millisecond)
	prometheus := s.Grafana.Datasource(org, "prometheus")
	require.Equal(t, "http://controlplane:8080/telemetry/places/prometheus", prometheus["url"])
	require.Equal(t, telemetryAuth("places")["Authorization"], prometheus["secureJsonData"].(map[string]any)["httpHeaderValue1"])
	require.Equal(t, "http://controlplane:8080/telemetry/places/loki", s.Grafana.Datasource(org, "loki")["url"])
}

func Test_TheEnvironmentPageEmbedsTheTeamsOwnOrg(t *testing.T) {
	s, p := setupPlaces(t)
	org := orgOf(t, s, "team-places")

	page := p.bob.Do(t, http.MethodGet, "/services/places-api/env/dev", nil)

	require.Equal(t, http.StatusOK, page.Status)
	require.Contains(t, page.Body, fmt.Sprintf(`/grafana/d-solo/platform-places-api?orgId=%d&panelId=1&var-env=dev`, org))
	require.Contains(t, page.Body, fmt.Sprintf(`/grafana/d/platform-places-api?orgId=%d&var-env=dev`, org))
}

func Test_LeavingATeamTakesThePersonOutOfItsOrg(t *testing.T) {
	s, p := setupPlaces(t)
	org := orgOf(t, s, "team-places")
	require.Eventually(t, func() bool { return s.Grafana.Members(org)["bob"] == "Viewer" }, 5*time.Second, 50*time.Millisecond)

	p.mia.MustJSON(t, http.MethodDelete, "/api/v1/teams/places/members/bob", nil, http.StatusNoContent, nil)

	require.Eventually(t, func() bool { _, in := s.Grafana.Members(org)["bob"]; return !in }, 5*time.Second, 50*time.Millisecond)
}

func Test_DeletingATeamDeletesItsOrg(t *testing.T) {
	s, p := setupPlaces(t)
	p.root.MustJSON(t, http.MethodPost, "/api/v1/teams", map[string]string{"slug": "other", "name": "Other"}, http.StatusCreated, nil)
	orgOf(t, s, "team-other")

	p.root.MustJSON(t, http.MethodDelete, "/api/v1/teams/other", nil, http.StatusNoContent, nil)

	require.Eventually(t, func() bool { _, ok := s.Grafana.OrgID("team-other"); return !ok }, 5*time.Second, 50*time.Millisecond)
}

func Test_OnlyAdminsKeepARoleInTheMainOrg(t *testing.T) {
	s, p := setupPlaces(t)
	orgOf(t, s, "team-places")
	s.Grafana.SetMember(1, "bob", "Viewer")

	p.mia.MustJSON(t, http.MethodPut, "/api/v1/teams/places/members/eve", map[string]string{"role": "member"}, http.StatusOK, nil)

	require.Eventually(t, func() bool { return s.Grafana.Members(1)["bob"] == "None" }, 5*time.Second, 50*time.Millisecond)
	require.Equal(t, "Admin", s.Grafana.Members(1)["root"])
}

func Test_TheDatabasePageEmbedsItsTeamsDatabaseDashboard(t *testing.T) {
	s, p, _ := setupDatabases(t)
	org := orgOf(t, s, "team-places")
	p.bob.Form(t, "/resources/databases", databaseForm(p.bob.CSRFToken))

	page := p.bob.Do(t, http.MethodGet, databasePage, nil)

	require.Equal(t, http.StatusOK, page.Status, page.Body)
	require.Contains(t, page.Body, fmt.Sprintf(`/grafana/d-solo/kalzone-database?from=now-6h&amp;orgId=%d&amp;panelId=4`, org))
	require.Contains(t, page.Body, "var-schema="+devSchema)
	require.Contains(t, page.Body, "var-user=kz_"+devSchema)
	require.Contains(t, page.Body, fmt.Sprintf(`/grafana/d/kalzone-database?orgId=%d&amp;var-schema=`+devSchema, org))
}

func Test_TheQueuePageEmbedsItsTeamsQueueDashboard(t *testing.T) {
	s, p := setupQueue(t)
	org := orgOf(t, s, "team-places")
	q := createQueue(t, p.bob, "dev", http.StatusCreated)

	page := p.bob.Do(t, http.MethodGet, "/resources/queues/dev/place-import", nil)

	require.Equal(t, http.StatusOK, page.Status, page.Body)
	require.Contains(t, page.Body, fmt.Sprintf(`/grafana/d-solo/kalzone-queue?from=now-6h&amp;orgId=%d&amp;panelId=1`, org))
	require.Contains(t, page.Body, fmt.Sprintf(`/grafana/d/kalzone-queue?orgId=%d&amp;var-queue_id=%s`, org, q.ID))
}
