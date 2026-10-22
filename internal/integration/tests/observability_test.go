package tests

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/fakegrafana"
	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/stretchr/testify/require"
)

const observabilityCard = "/services/places-api/env/dev/observability"

func withTelemetry(s *integration.Suite) {
	s.Grafana.SetSeries("increase(traefik_service_requests_total", 1200, 800, 2371)
	s.Grafana.SetSeries(`code=~"5.."`, 0.002, 0.049, 0.002)
	s.Grafana.SetSeries("request_duration_seconds_bucket", 120, 489, 168)
	s.Grafana.SetSeries("container_memory_working_set_bytes", 200, 212)
	s.Grafana.SetLines(
		fakegrafana.LokiLine{Container: "places-api-dev-blue", At: time.Now().Add(-time.Minute), Line: "error: mysql: too many connections"},
		fakegrafana.LokiLine{Container: "places-api-dev-blue", At: time.Now(), Line: "GET /places 200 41ms"},
	)
}

func Test_TheEnvironmentPageLoadsItsObservabilityCard(t *testing.T) {
	_, p := setupPlaces(t)

	page := p.bob.Do(t, http.MethodGet, "/services/places-api/env/dev", nil)

	require.Equal(t, http.StatusOK, page.Status)
	require.Contains(t, page.Body, `hx-get="/services/places-api/env/dev/observability"`)
	require.NotContains(t, page.Body, "/grafana/")
}

func Test_TheObservabilityCardDrawsTheServicesOwnMetricsAndLogs(t *testing.T) {
	s, p := setupPlaces(t)
	withTelemetry(s)

	card := p.bob.Do(t, http.MethodGet, observabilityCard+"?window=6h&signal=latency", nil)

	require.Equal(t, http.StatusOK, card.Status, card.Body)
	require.Contains(t, card.Body, "4,371")
	require.Contains(t, card.Body, "peak 4.9%")
	require.Contains(t, card.Body, "168 ms")
	require.Contains(t, card.Body, "212 MB")
	require.Contains(t, card.Body, "peak 489 ms at")
	require.Contains(t, card.Body, "error: mysql: too many connections")
	require.Less(t, strings.Index(card.Body, "GET /places 200 41ms"), strings.Index(card.Body, "too many connections"))
	require.NotContains(t, card.Body, "/grafana/")
}

func Test_TheErrorsFilterAsksLokiForErrorLinesOnly(t *testing.T) {
	s, p := setupPlaces(t)
	withTelemetry(s)

	card := p.bob.Do(t, http.MethodGet, observabilityCard+"?logs=errors", nil)
	queries := s.Grafana.LokiQueries()

	require.Equal(t, http.StatusOK, card.Status, card.Body)
	require.Contains(t, queries[len(queries)-1], `places-api-dev-(blue|green)`)
	require.Contains(t, queries[len(queries)-1], `|~ "(?i)(error|panic`)
}

func Test_TheObservabilityCardIsHiddenFromOtherTeams(t *testing.T) {
	s, p := setupPlaces(t)
	withTelemetry(s)

	outsider := p.eve.Do(t, http.MethodGet, observabilityCard, nil)

	require.Equal(t, http.StatusNotFound, outsider.Status)
}

func Test_OnlyAdminsGetALinkToTheGrafanaDashboard(t *testing.T) {
	s, p := setupPlaces(t)
	withTelemetry(s)

	member := p.bob.Do(t, http.MethodGet, observabilityCard, nil)
	admin := p.root.Do(t, http.MethodGet, observabilityCard, nil)

	require.NotContains(t, member.Body, "/grafana/d/")
	require.Contains(t, admin.Body, `/grafana/d/platform-places-api?var-env=dev`)
}
