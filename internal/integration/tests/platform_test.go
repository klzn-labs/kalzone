package tests

import (
	"net/http"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/klzn-labs/kalzone/pkg/agentapi"
	"github.com/stretchr/testify/require"
)

func controlPlaneReport() agentapi.PlatformReport {
	at := time.Now().UTC().Add(-time.Hour)
	return agentapi.PlatformReport{
		Live:      agentapi.PlatformRelease{Version: "c63f05ec82a612925be093856d8b0a1228c28727+cfg.4f2232a4", Image: "img:c63f05e", Colour: "green", UpdatedAt: at},
		Container: agentapi.Container{Found: true, Name: "controlplane-green", State: "running", Health: "healthy"},
		History: []agentapi.PlatformDeploy{
			{At: at, Version: "c63f05e+cfg.4f2232a4", Outcome: "deployed", Strategy: "bluegreen", Colour: "green", RunURL: "https://github.com/run/2"},
			{At: at.Add(-10 * time.Minute), Version: "c63f05e+cfg.4f2232a4", Outcome: "rolled_back", Step: "gate", Reason: "p95 latency 331ms above limit 293ms (baseline 93ms)", RunURL: "https://github.com/run/1"},
		},
	}
}

func reportControlPlane(t *testing.T, s *integration.Suite) {
	t.Helper()
	resp := s.Agent(t, http.MethodPut, agentapi.PlatformPath("controlplane"), controlPlaneReport())
	require.Equal(t, http.StatusNoContent, resp.Status, resp.Body)
}

func Test_TheControlPlaneShowsReadOnlyInTheCatalog(t *testing.T) {
	s, p := setupPlaces(t)
	reportControlPlane(t, s)

	catalog := p.eve.Do(t, http.MethodGet, "/services", nil)
	page := p.eve.Do(t, http.MethodGet, "/platform/controlplane", nil)

	require.Equal(t, http.StatusOK, catalog.Status)
	require.Contains(t, catalog.Body, `href="/platform/controlplane"`)
	require.Contains(t, catalog.Body, `href="/services?filter=platform"`)
	require.Equal(t, http.StatusOK, page.Status)
	require.Contains(t, page.Body, "c63f05e")
	require.Contains(t, page.Body, "p95 latency 331ms above limit 293ms (baseline 93ms)")
	require.Contains(t, page.Body, "https://github.com/run/1")
	require.NotContains(t, page.Body, `action="/platform`)
}

func Test_ThePlatformTabShowsOnlyTheControlPlane(t *testing.T) {
	s, p := setupPlaces(t)
	reportControlPlane(t, s)

	platformTab := p.eve.Do(t, http.MethodGet, "/services?filter=platform", nil)
	mine := p.bob.Do(t, http.MethodGet, "/services?filter=mine", nil)

	require.Contains(t, platformTab.Body, `href="/platform/controlplane"`)
	require.NotContains(t, platformTab.Body, `href="/services/places-api"`)
	require.NotContains(t, mine.Body, `href="/platform/controlplane"`)
	require.Contains(t, mine.Body, `href="/services/places-api"`)
}

func Test_TheCatalogWorksBeforeTheControlPlaneIsReported(t *testing.T) {
	_, p := setupPlaces(t)

	catalog := p.eve.Do(t, http.MethodGet, "/services", nil)
	page := p.eve.Do(t, http.MethodGet, "/platform/controlplane", nil)

	require.Equal(t, http.StatusOK, catalog.Status)
	require.NotContains(t, catalog.Body, `href="/platform/controlplane"`)
	require.Equal(t, http.StatusNotFound, page.Status)
}

func Test_TheAgentCannotReportUnknownPlatformComponents(t *testing.T) {
	s, _ := setupPlaces(t)

	resp := s.Agent(t, http.MethodPut, agentapi.PlatformPath("places-api"), controlPlaneReport())

	require.Equal(t, http.StatusNotFound, resp.Status, resp.Body)
}
