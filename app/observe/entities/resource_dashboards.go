package entities

import "fmt"

const (
	DatabaseDashboardUID = "kalzone-database"
	QueueDashboardUID    = "kalzone-queue"

	PanelDatabaseQueries     = 1
	PanelDatabaseLatency     = 2
	PanelDatabaseConnections = 3
	PanelDatabaseLoad        = 4
	PanelDatabaseCPU         = 5
	PanelDatabaseSlow        = 6

	PanelQueueWaiting    = 1
	PanelQueueDeliveries = 2
	PanelQueueLatency    = 3
	PanelQueueRate       = 4
	PanelQueueInFlight   = 5
	PanelQueueDead       = 6

	databaseSchema = `{schema="$schema"}`
	queueSelector  = `{queue_id="$queue_id"}`
)

var EmbeddedResourcePanels = []int{1, 2, 3, 4}

func DatabaseDashboard() map[string]any {
	statementsOf := func(counter string) string {
		return fmt.Sprintf("sum(rate(%s_%s%s[$__rate_interval]))", statements, counter, databaseSchema)
	}
	return resourceDashboard(DatabaseDashboardUID, "Database", []string{"schema", "user"}, []any{
		timeseries(PanelDatabaseQueries, "Queries per second", "reqps", 0, 0, statementsOf("total")),
		timeseries(PanelDatabaseLatency, "Average query time", "s", 8, 0, statementsOf("seconds_total")+" / "+statementsOf("total")),
		timeseries(PanelDatabaseConnections, "Connections", "none", 16, 0, `sum(mysql_info_schema_processlist_processes_by_user{mysql_user="$user"})`),
		timeseries(PanelDatabaseLoad, "Time in queries", "s", 0, 8, statementsOf("seconds_total")),
		placed(mysqlCPU(0), PanelDatabaseCPU, 8, 8, 16),
		placed(slowQueries(fmt.Sprintf("group by (schema) (%s_total%s)", statements, databaseSchema), 0), PanelDatabaseSlow, 0, 16, 24),
	})
}

func QueueDashboard() map[string]any {
	return resourceDashboard(QueueDashboardUID, "Queue", []string{"queue_id"}, []any{
		timeseries(PanelQueueWaiting, "Waiting", "short", 0, 0, "sum(platform_queue_depth"+queueSelector+")"),
		legend(timeseries(PanelQueueDeliveries, "Deliveries per second", "reqps", 8, 0,
			"sum by (outcome) (rate(platform_queue_delivered_total"+queueSelector+"[$__rate_interval]))"), "{{outcome}}"),
		timeseries(PanelQueueLatency, "Delivery p95", "s", 16, 0,
			"histogram_quantile(0.95, sum by (le) (rate(platform_queue_delivery_seconds_bucket"+queueSelector+"[5m])))"),
		timeseries(PanelQueueRate, "Rate allowed now", "reqps", 0, 8, "max(platform_queue_effective_rate"+queueSelector+")"),
		timeseries(PanelQueueInFlight, "In flight", "short", 8, 8, "sum(platform_queue_in_flight"+queueSelector+")"),
		timeseries(PanelQueueDead, "Dead letters", "short", 16, 8, "sum(platform_queue_dead_letters"+queueSelector+")"),
	})
}

func resourceDashboard(uid, title string, variables []string, panels []any) map[string]any {
	return map[string]any{
		"uid": uid, "title": title, "tags": []string{"platform", "resource"},
		"timezone": "browser", "refresh": "30s", "schemaVersion": 39,
		"time":       map[string]string{"from": "now-6h", "to": "now"},
		"templating": map[string]any{"list": hiddenVariables(variables)},
		"panels":     panels,
	}
}

func hiddenVariables(names []string) []any {
	list := make([]any, 0, len(names))
	for _, name := range names {
		list = append(list, map[string]any{"name": name, "type": "textbox", "hide": 2, "query": ""})
	}
	return list
}

func placed(panel map[string]any, id, x, y, w int) map[string]any {
	panel["id"] = id
	grid := panel["gridPos"].(map[string]int)
	grid["x"], grid["y"], grid["w"] = x, y, w
	return panel
}
