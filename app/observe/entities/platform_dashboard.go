package entities

import "fmt"

const (
	PanelRouteRequests = 14
	PanelRouteLatency  = 15
	PanelGoroutines    = 16
	PanelHeap          = 17

	platformJob    = "controlplane"
	internalRoutes = `route!~"/metrics|/healthz"`
	runtimeRowY    = 16
	platformLogsY  = 32
)

var PlatformComponents = []string{"controlplane"}

func PlatformDashboardUID(name string) string {
	return "kalzone-" + name
}

func PlatformDashboard(name string) map[string]any {
	return map[string]any{
		"uid": PlatformDashboardUID(name), "title": "Kalzone " + name, "tags": []string{"kalzone", name},
		"timezone": "browser", "refresh": "30s", "schemaVersion": 39,
		"time":   map[string]string{"from": "now-6h", "to": "now"},
		"panels": platformPanels(name),
	}
}

func platformPanels(name string) []any {
	all := append(workloadPanels(name), runtimePanels()...)
	return append(all, logs(PanelLogs, platformLogsY, fmt.Sprintf(`{container=~"%s-(blue|green)"}`, name)))
}

func runtimePanels() []any {
	job := fmt.Sprintf(`job="%s"`, platformJob)
	requests := fmt.Sprintf("platform_http_request_duration_seconds_count{%s,%s}", job, internalRoutes)
	buckets := fmt.Sprintf("platform_http_request_duration_seconds_bucket{%s,%s}", job, internalRoutes)
	return []any{
		legend(counted(timeseries(PanelRouteRequests, "Requests by route", "short", 0, runtimeRowY, fmt.Sprintf(
			"topk(8, sum by (route) (increase(%s[$__interval])))", requests))), "{{route}}"),
		legend(timeseries(PanelRouteLatency, "p95 by route", "s", 8, runtimeRowY, fmt.Sprintf(
			"topk(8, histogram_quantile(0.95, sum by (le, route) (rate(%s[5m]))))", buckets)), "{{route}}"),
		timeseries(PanelGoroutines, "Goroutines", "short", 16, runtimeRowY, fmt.Sprintf("sum(go_goroutines{%s})", job)),
		timeseries(PanelHeap, "Heap in use", "bytes", 0, runtimeRowY+8, fmt.Sprintf("sum(go_memstats_heap_inuse_bytes{%s})", job)),
	}
}

func legend(panel map[string]any, format string) map[string]any {
	panel["targets"].([]map[string]any)[0]["legendFormat"] = format
	return panel
}
