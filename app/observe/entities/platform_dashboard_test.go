package entities_test

import (
	"encoding/json"
	"testing"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/stretchr/testify/require"
)

func Test_PlatformDashboard_QueriesTheControlPlaneWithoutAnEnvironment(t *testing.T) {
	raw, err := json.Marshal(entities.PlatformDashboard("controlplane"))
	require.NoError(t, err)
	body := string(raw)

	require.Contains(t, body, `"uid":"kalzone-controlplane"`)
	require.Contains(t, body, `service=~\"controlplane-(blue|green)@file\"`)
	require.Contains(t, body, `name=~\"controlplane-(blue|green)\"`)
	require.Contains(t, body, `{container=~\"controlplane-(blue|green)\"}`)
	require.Contains(t, body, `kalzone_traffic_weight_percent{service=\"controlplane\"}`)
	require.Contains(t, body, `platform_http_request_duration_seconds_count{job=\"controlplane\",route!~\"/metrics|/healthz\"}`)
	require.Contains(t, body, `go_goroutines{job=\"controlplane\"}`)
	require.NotContains(t, body, "${env}")
	require.NotContains(t, body, "templating")
}

func Test_PlatformDashboard_KeepsPanelIDsUnique(t *testing.T) {
	seen := map[int]bool{}
	for _, p := range entities.PlatformDashboard("controlplane")["panels"].([]any) {
		id := p.(map[string]any)["id"].(int)
		require.False(t, seen[id], "panel id %d repeated", id)
		seen[id] = true
	}
}
