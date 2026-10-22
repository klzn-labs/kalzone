package tests

import (
	"net/http"
	"strings"
	"testing"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/klzn-labs/kalzone/pkg/agentapi"
	"github.com/stretchr/testify/require"
)

func setupDemo(t *testing.T) (*integration.Suite, people) {
	t.Helper()
	return setupPlaces(t, integration.WithDemo)
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

func Test_AGuestSeesOnlyTheDemoTeam(t *testing.T) {
	s, _ := setupDemo(t)
	guest := s.Guest(t)
	var services serviceList

	guest.MustJSON(t, http.MethodGet, "/api/v1/services", nil, http.StatusOK, &services)
	overview := guest.Do(t, http.MethodGet, "/", nil)
	hidden := guest.Do(t, http.MethodGet, "/api/v1/services/places-api", nil)

	require.Len(t, services.Services, 4)
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
	card := guest.Do(t, http.MethodGet, "/services/demo-shop-api/env/prod/observability", nil)

	require.Equal(t, http.StatusOK, prod.Status)
	require.Contains(t, prod.Body, "opens a change request that a maintainer approves")
	require.NotContains(t, prod.Body, `action="/services/demo-shop-api/env/prod/deployments"`)
	require.Contains(t, releases.Body, "In your own team, Build runs")
	require.Contains(t, releases.Body, "r18")
	require.Equal(t, http.StatusOK, card.Status)
	require.Contains(t, card.Body, "p95 latency")
	require.NotContains(t, card.Body, "No data in this window")
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
	for _, prefix := range []string{"/logout", "/demo", "/agent/", "/queued/", "/grafana/", "/webhooks/", "/mcp", "/api/v1/releases/:id/"} {
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
	require.Equal(t, http.StatusConflict, build.Status, build.Body)
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
