package entities_test

import (
	"encoding/json"
	"testing"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/stretchr/testify/require"
)

func Test_TheDatabaseDashboardFiltersEveryPanelByItsSchemaOrUser(t *testing.T) {
	body, err := json.Marshal(entities.DatabaseDashboard())

	require.NoError(t, err)
	require.Contains(t, string(body), `mysql_perf_schema_events_statements_total{schema=\"$schema\"}`)
	require.Contains(t, string(body), `mysql_info_schema_processlist_processes_by_user{mysql_user=\"$user\"}`)
	require.Contains(t, string(body), `"name":"schema"`)
}

func Test_TheQueueDashboardFiltersEveryPanelByTheQueueID(t *testing.T) {
	d := entities.QueueDashboard()

	for _, panel := range d["panels"].([]any) {
		expr := panel.(map[string]any)["targets"].([]map[string]any)[0]["expr"].(string)
		require.Contains(t, expr, `queue_id="$queue_id"`)
	}
}

func Test_TheEmbeddedPanelsExistOnBothResourceDashboards(t *testing.T) {
	for _, d := range []map[string]any{entities.DatabaseDashboard(), entities.QueueDashboard()} {
		ids := map[int]bool{}
		for _, panel := range d["panels"].([]any) {
			ids[panel.(map[string]any)["id"].(int)] = true
		}
		for _, id := range entities.EmbeddedResourcePanels {
			require.True(t, ids[id], "%s lacks panel %d", d["uid"], id)
		}
	}
}
