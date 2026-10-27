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
	require.Equal(t, []string{"MySQL CPU (shared)", "Connections", "Queries per second", "Slowest queries"}, embeddedTitles(entities.DatabaseDashboard(), entities.DatabaseEmbeds))
	require.Equal(t, []string{"Waiting", "Deliveries per second", "Delivery p95", "Rate allowed now"}, embeddedTitles(entities.QueueDashboard(), entities.QueueEmbeds))
}

func embeddedTitles(d map[string]any, embeds []entities.EmbeddedPanel) []string {
	titles := map[int]string{}
	for _, panel := range d["panels"].([]any) {
		p := panel.(map[string]any)
		titles[p["id"].(int)] = p["title"].(string)
	}
	result := []string{}
	for _, e := range embeds {
		result = append(result, titles[e.ID])
	}
	return result
}
