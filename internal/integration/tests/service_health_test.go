package tests

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const p95 = "traefik_service_request_duration_seconds_bucket"

type limitsView struct {
	Limits struct {
		ResponseMS   int `json:"response_ms"`
		CPUPercent   int `json:"cpu_percent"`
		DBCPUPercent int `json:"db_cpu_percent"`
	} `json:"limits"`
}

func Test_AFailedBuildBlinksInTheCatalog(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)
	var rel release
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/releases", map[string]string{"service": "places-api", "ref": "main"}, http.StatusAccepted, &rel)
	callback(t, s, rel.ID, "failed", repo, map[string]string{"failure": "go test failed", "run_url": runURL})

	catalog := p.bob.Do(t, http.MethodGet, "/services", nil)
	page := p.bob.Do(t, http.MethodGet, "/services/places-api", nil)

	require.Contains(t, catalog.Body, `class="alarm"`)
	require.Contains(t, catalog.Body, "Build #1 failed")
	require.Contains(t, catalog.Body, "1 needs attention")
	require.Contains(t, page.Body, "Build #1 failed")
}

func Test_AMetricOverItsLimitBlinksUntilTheServiceRaisesItsLimit(t *testing.T) {
	s, p := setupPlaces(t)
	s.Grafana.SetSeries(p95, 210, 260, 450, 440, 460, 430, 450)

	before := p.bob.Do(t, http.MethodGet, "/services", nil)
	overview := p.bob.Do(t, http.MethodGet, "/services/places-api", nil)
	var raised limitsView
	p.bob.MustJSON(t, http.MethodPatch, "/api/v1/services/places-api", map[string]any{"limits": map[string]int{"response_ms": 500}}, http.StatusOK, &raised)
	after := p.bob.Do(t, http.MethodGet, "/services", nil)
	calm := p.bob.Do(t, http.MethodGet, "/services/places-api", nil)

	require.Contains(t, before.Body, "Response p95 over 300 ms")
	require.Contains(t, overview.Body, "Response p95 is over 300 ms")
	require.Contains(t, overview.Body, `class="metric-chart"`)
	require.Equal(t, 500, raised.Limits.ResponseMS)
	require.NotContains(t, after.Body, `class="alarm"`)
	require.Contains(t, calm.Body, "Prod is healthy")
	require.Contains(t, calm.Body, "<b>450 ms</b>")
}

func Test_AShortSpikeOverTheLimitDoesNotBlink(t *testing.T) {
	s, p := setupPlaces(t)
	s.Grafana.SetSeries(p95, 210, 260, 230, 250, 450, 470)

	catalog := p.bob.Do(t, http.MethodGet, "/services", nil)
	overview := p.bob.Do(t, http.MethodGet, "/services/places-api", nil)

	require.NotContains(t, catalog.Body, `class="alarm"`)
	require.Contains(t, overview.Body, "Prod is healthy")
	require.Contains(t, overview.Body, "<b>470 ms</b>")
}

func Test_AServiceWithoutSamplesSaysSoAndIsNeverBroken(t *testing.T) {
	_, p := setupPlaces(t)

	catalog := p.bob.Do(t, http.MethodGet, "/services", nil)
	overview := p.bob.Do(t, http.MethodGet, "/services/places-api", nil)

	require.NotContains(t, catalog.Body, `class="alarm"`)
	require.Contains(t, overview.Body, "No metrics yet")
}

func Test_TheEditFormSetsAndClearsLimits(t *testing.T) {
	_, p := setupPlaces(t)
	form := func(value string) url.Values {
		return url.Values{"_csrf": {p.bob.CSRFToken}, "limit_response_ms": {value}, "limit_cpu_percent": {""}, "limit_db_cpu_percent": {"150"}}
	}
	var saved, cleared limitsView

	set := p.bob.Form(t, "/services/places-api/update", form("800"))
	p.bob.MustJSON(t, http.MethodGet, "/api/v1/services/places-api", nil, http.StatusOK, &saved)
	p.bob.Form(t, "/services/places-api/update", form(""))
	p.bob.MustJSON(t, http.MethodGet, "/api/v1/services/places-api", nil, http.StatusOK, &cleared)
	refused := p.bob.Do(t, http.MethodPatch, "/api/v1/services/places-api", map[string]any{"limits": map[string]int{"cpu_percent": 101}})

	require.Less(t, set.Status, 400, set.Body)
	require.Equal(t, 800, saved.Limits.ResponseMS)
	require.Equal(t, 150, saved.Limits.DBCPUPercent)
	require.Equal(t, 0, cleared.Limits.ResponseMS)
	require.Equal(t, http.StatusBadRequest, refused.Status, refused.Body)
	require.True(t, strings.Contains(refused.Body, "limits.cpu_percent"), refused.Body)
}
