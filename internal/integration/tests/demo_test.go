package tests

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/klzn-labs/kalzone/pkg/agentapi"
	"github.com/stretchr/testify/require"
)

func setupDemo(t *testing.T) (*integration.Suite, people) {
	t.Helper()
	s, p := setupPlaces(t, integration.WithDemo)
	s.PlantDemo(t)
	return s, p
}

func Test_TheLoginPageOffersTheDemoOnlyWhenItIsOpen(t *testing.T) {
	open, _ := setupDemo(t)
	closed := setupSuiteOnly(t)

	withDemo := open.Anonymous(t).Do(t, http.MethodGet, "/login", nil)
	without := closed.Anonymous(t).Do(t, http.MethodGet, "/login", nil)
	refused := closed.Anonymous(t).Do(t, http.MethodPost, "/demo", nil)

	require.Contains(t, withDemo.Body, "Open the demo")
	require.Contains(t, withDemo.Body, "Continue with GitHub")
	require.NotContains(t, without.Body, "Open the demo")
	require.Equal(t, "/login?error=demo", refused.Location)
}

func Test_TheDemoTeamHasOnlyItsMember(t *testing.T) {
	s, _ := setupDemo(t)
	var members struct {
		Items []struct {
			Login string `json:"login"`
		} `json:"items"`
	}

	s.Guest(t).MustJSON(t, http.MethodGet, "/api/v1/teams/demo/members", nil, http.StatusOK, &members)

	require.Len(t, members.Items, 1)
	require.Equal(t, integration.DemoMember, members.Items[0].Login)
}

func Test_AGuestSeesOnlyTheDemoTeam(t *testing.T) {
	s, _ := setupDemo(t)
	guest := s.Guest(t)
	var services serviceList

	guest.MustJSON(t, http.MethodGet, "/api/v1/services", nil, http.StatusOK, &services)
	overview := guest.Do(t, http.MethodGet, "/", nil)
	hidden := guest.Do(t, http.MethodGet, "/api/v1/services/places-api", nil)

	require.Len(t, services.Services, 1)
	for _, svc := range services.Services {
		require.True(t, strings.HasPrefix(svc.Slug, "demo-"), svc.Slug)
	}
	require.Contains(t, overview.Body, "Shop API")
	require.Contains(t, overview.Body, `class="demo-tag"`)
	require.Contains(t, overview.Body, "Leave the demo")
	require.NotContains(t, overview.Body, `href="/tokens"`)
	require.NotContains(t, overview.Body, "You're not on a team yet")
	require.Equal(t, http.StatusNotFound, hidden.Status)
}

func Test_TheDemoTeachesWhatWouldHappenInYourOwnTeam(t *testing.T) {
	s, _ := setupDemo(t)
	guest := s.Guest(t)

	prod := guest.Do(t, http.MethodGet, "/services/demo-shop-api/env/prod", nil)
	releases := guest.Do(t, http.MethodGet, "/services/demo-shop-api/releases", nil)

	require.Equal(t, http.StatusOK, prod.Status)
	require.Contains(t, prod.Body, "opens a change request that a maintainer approves")
	require.NotContains(t, prod.Body, `action="/services/demo-shop-api/env/prod/deployments"`)
	require.Contains(t, releases.Body, "In your own team, Build runs")
	require.Contains(t, releases.Body, "r18")
}

func Test_TheDemoEmbedsGrafanasDemoOrgOfTestData(t *testing.T) {
	s, _ := setupDemo(t)
	guest := s.Guest(t)
	org := orgOf(t, s, "demo")
	require.Eventually(t, func() bool { return s.Grafana.OrgDashboards(org)["platform-demo-shop-api"] }, 5*time.Second, 50*time.Millisecond)

	prod := guest.Do(t, http.MethodGet, "/services/demo-shop-api/env/prod", nil)

	require.Contains(t, prod.Body, fmt.Sprintf("/grafana/d-solo/platform-demo-shop-api?orgId=%d&panelId=1", org))
	require.Equal(t, "Viewer", s.Grafana.Members(org)["guest.demo"])
	require.Equal(t, "grafana-testdata-datasource", s.Grafana.Datasource(org, "testdata")["type"])
	require.Nil(t, s.Grafana.Datasource(org, "prometheus"))
}

func Test_AGuestCanChangeNothing(t *testing.T) {
	s, _ := setupDemo(t)
	guest := s.Guest(t)

	for _, route := range s.Engine.Routes() {
		if route.Method == http.MethodGet || route.Method == http.MethodHead || notGuarded(route.Path) {
			continue
		}
		resp := guest.Do(t, route.Method, concrete(route.Path), map[string]any{})
		require.Equal(t, http.StatusForbidden, resp.Status, route.Method+" "+route.Path+": "+resp.Body)
	}
}

func concrete(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			parts[i] = "demo-shop-api"
		}
	}
	return strings.Join(parts, "/")
}

func notGuarded(path string) bool {
	for _, prefix := range []string{"/logout", "/demo", "/agent/", "/queued/", "/grafana/", "/telemetry/", "/webhooks/", "/mcp", "/api/v1/releases/:id/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func Test_DemoServicesNeverDeployEvenForAnAdmin(t *testing.T) {
	s, p := setupDemo(t)

	refused := p.root.Do(t, http.MethodPost, "/api/v1/deployments/config-applications", map[string]string{"service": "demo-shop-api", "environment": "dev"})
	build := p.root.Do(t, http.MethodPost, "/api/v1/releases", map[string]string{"service": "demo-shop-api", "ref": "main"})
	desired := s.Agent(t, http.MethodGet, agentapi.DesiredPath, nil)

	require.Equal(t, http.StatusConflict, refused.Status, refused.Body)
	require.Equal(t, http.StatusForbidden, build.Status, build.Body)
	require.Equal(t, http.StatusOK, desired.Status)
	require.NotContains(t, desired.Body, "demo-")
}

func Test_ANewcomerAlsoSeesTheDemoTeam(t *testing.T) {
	_, p := setupDemo(t)
	var teams struct {
		Items []struct {
			Slug string `json:"slug"`
		} `json:"items"`
	}

	p.eve.MustJSON(t, http.MethodGet, "/api/v1/teams", nil, http.StatusOK, &teams)
	overview := p.eve.Do(t, http.MethodGet, "/", nil)

	require.Len(t, teams.Items, 1)
	require.Equal(t, "demo", teams.Items[0].Slug)
	require.Contains(t, overview.Body, "You're not on a team yet")
}

func Test_TeamMembersDoNotSeeTheDemoTeam(t *testing.T) {
	_, p := setupDemo(t)
	var services serviceList

	p.bob.MustJSON(t, http.MethodGet, "/api/v1/services", nil, http.StatusOK, &services)
	demoTeam := p.bob.Do(t, http.MethodGet, "/api/v1/teams/demo", nil)

	require.Len(t, services.Services, 1)
	require.Equal(t, "places-api", services.Services[0].Slug)
	require.Equal(t, http.StatusNotFound, demoTeam.Status)
}

func Test_TheDemoDoorIsRateLimitedPerAddress(t *testing.T) {
	s, _ := setupDemo(t)
	locations := []string{}

	for range 6 {
		locations = append(locations, s.Anonymous(t).Do(t, http.MethodPost, "/demo", nil).Location)
	}

	require.Equal(t, "/", locations[4])
	require.Equal(t, "/login?error=demo-busy", locations[5])
}

func Test_TheDemosMemberCannotDecideItsWaitingRequest(t *testing.T) {
	s, p := setupDemo(t)
	var listed struct {
		Items []requestView `json:"items"`
	}
	s.Guest(t).MustJSON(t, http.MethodGet, "/api/v1/requests?status=pending", nil, http.StatusOK, &listed)
	require.Len(t, listed.Items, 1)
	waiting := pendingChange{ID: listed.Items[0].ID, Poll: "/api/v1/requests/" + listed.Items[0].ID}

	approve := decide(t, p.root, waiting, "approve", url.Values{})
	reject := decide(t, p.root, waiting, "reject", url.Values{"reason": {"no"}})

	require.Equal(t, http.StatusUnprocessableEntity, approve.Status, approve.Body)
	require.Contains(t, approve.Body, "the demo is read-only")
	require.Equal(t, http.StatusUnprocessableEntity, reject.Status, reject.Body)
	require.Equal(t, "pending", poll(t, p.root, waiting).Status)
}
