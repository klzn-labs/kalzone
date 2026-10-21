package entities_test

import (
	"encoding/json"
	"testing"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/stretchr/testify/require"
)

func Test_AccessLog_ReadsTheServicesTraefikLinesWithIDsCollapsed(t *testing.T) {
	query := entities.AccessLog("api-${env}")

	require.Contains(t, query, `{container="plateful-traefik-1"} |= "RequestPath" | json | __error__=""`)
	require.Contains(t, query, `ServiceName=~"api-${env}-(blue|green)@file"`)
	require.Contains(t, query, "regexReplaceAll `/[0-9]+\\\\b`")
	require.Contains(t, query, "regexReplaceAll `/[0-9a-fA-F-]{16,}\\\\b` .RequestPath `/:id`")
}

func Test_Dashboard_CountsRequestsInsteadOfRates(t *testing.T) {
	body := dashboardJSON(t, entities.Service{Slug: "web", Name: "Web"})

	require.Contains(t, body, `"title":"Requests"`)
	require.Contains(t, body, `sum(increase(traefik_service_requests_total{service=~\"web-${env}-(blue|green)@file\"}[$__interval]))`)
	require.Contains(t, body, `"calcs":["sum"]`)
	require.Contains(t, body, `"drawStyle":"bars"`)
	require.NotContains(t, body, `"reqps"`)
}

func Test_Dashboard_HasRequestsAndP95ByRouteFromTheAccessLog(t *testing.T) {
	body := dashboardJSON(t, entities.Service{Slug: "web", Name: "Web"})

	require.Contains(t, body, `"title":"Requests by route"`)
	require.Contains(t, body, `topk(8, sum by (route) (count_over_time({container=\"plateful-traefik-1\"}`)
	require.Contains(t, body, `"title":"p95 by route"`)
	require.Contains(t, body, `| unwrap Duration [$__interval]) by (route)) / 1e9`)
	require.Contains(t, body, `"gridPos":{"h":8,"w":12,"x":12,"y":16}`)
}

func Test_PlatformDashboard_CountsRequestsByRoute(t *testing.T) {
	raw, err := json.Marshal(entities.PlatformDashboard("controlplane"))
	require.NoError(t, err)

	require.Contains(t, string(raw), `topk(8, sum by (route) (increase(platform_http_request_duration_seconds_count{job=\"controlplane\",route!~\"/metrics|/healthz\"}[$__interval])))`)
	require.NotContains(t, string(raw), "reqps")
}
