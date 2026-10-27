package tests

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/klzn-labs/kalzone/app/observe/usecases/telemetry"
	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/stretchr/testify/require"
)

func telemetryAuth(team string) map[string]string {
	token := telemetry.NewTelemetryUsecase(integration.TelemetryKey, "", nil, nil, nil, nil).Token(team)
	return map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/x-www-form-urlencoded"}
}

func Test_ATeamsPrometheusDatasourceOnlyReadsItsOwnServices(t *testing.T) {
	s, _ := setupPlaces(t)
	form := url.Values{"query": {`sum(rate(traefik_service_requests_total[5m]))`}}

	answer := s.Raw(t, http.MethodPost, "/telemetry/places/prometheus/api/v1/query", []byte(form.Encode()), telemetryAuth("places"))

	require.Equal(t, http.StatusOK, answer.Status, answer.Body)
	received := s.Telemetry.Received()
	require.Len(t, received, 1)
	require.Equal(t, "/api/v1/query", received[0].Path)
	require.Equal(t, `sum(rate(traefik_service_requests_total{service=~"(?:places-api)-(?:dev|prod)-(?:blue|green)@file"}[5m]))`, received[0].Form.Get("query"))
	require.Empty(t, received[0].Auth)
}

func Test_ATeamsLokiDatasourceReadsOnlyItsServicesTenants(t *testing.T) {
	s, _ := setupPlaces(t)

	answer := s.Raw(t, http.MethodGet, `/telemetry/places/loki/loki/api/v1/query_range?query=%7Bservice%3D~%22.%2B%22%7D`, nil, telemetryAuth("places"))

	require.Equal(t, http.StatusOK, answer.Status, answer.Body)
	received := s.Telemetry.Received()
	require.Equal(t, "/loki-root/loki/api/v1/query_range", received[0].Path)
	require.Equal(t, `{service=~".+"}`, received[0].Form.Get("query"))
	require.Equal(t, "places-api-dev|places-api-prod", received[0].Tenants)
}

func Test_TelemetryRefusesAnotherTeamsTokenAndWrites(t *testing.T) {
	s, p := setupPlaces(t)
	p.root.MustJSON(t, http.MethodPost, "/api/v1/teams", map[string]string{"slug": "other", "name": "Other"}, http.StatusCreated, nil)

	stolen := s.Raw(t, http.MethodGet, "/telemetry/places/prometheus/api/v1/query?query=up", nil, telemetryAuth("other"))
	push := s.Raw(t, http.MethodPost, "/telemetry/places/loki/loki/api/v1/push", []byte("{}"), telemetryAuth("places"))
	anonymous := s.Raw(t, http.MethodGet, "/telemetry/places/prometheus/api/v1/query?query=up", nil, nil)

	require.Equal(t, http.StatusUnauthorized, stolen.Status, stolen.Body)
	require.Equal(t, http.StatusForbidden, push.Status, push.Body)
	require.Equal(t, http.StatusUnauthorized, anonymous.Status, anonymous.Body)
	require.Empty(t, s.Telemetry.Received())
}

func Test_ATeamWithoutServicesReadsNothing(t *testing.T) {
	s, p := setupPlaces(t)
	p.root.MustJSON(t, http.MethodPost, "/api/v1/teams", map[string]string{"slug": "other", "name": "Other"}, http.StatusCreated, nil)

	s.Raw(t, http.MethodGet, "/telemetry/other/prometheus/api/v1/query?query=container_memory_working_set_bytes", nil, telemetryAuth("other"))

	query := s.Telemetry.Received()[0].Form.Get("query")
	require.True(t, strings.Contains(query, `kalzone_out_of_scope="true"`), query)
}

func Test_ATeamsPrometheusDatasourceReadsItsOwnQueues(t *testing.T) {
	s, p := setupQueue(t)
	q := createQueue(t, p.bob, "dev", http.StatusCreated)
	form := url.Values{"query": {`platform_queue_depth`}}

	s.Raw(t, http.MethodPost, "/telemetry/places/prometheus/api/v1/query", []byte(form.Encode()), telemetryAuth("places"))

	require.Equal(t, `platform_queue_depth{queue_id=~"(?:`+q.ID+`)"}`, s.Telemetry.Received()[0].Form.Get("query"))
}
