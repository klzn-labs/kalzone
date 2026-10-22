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
