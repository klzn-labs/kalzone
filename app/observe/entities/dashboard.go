package entities

import "fmt"

const (
	FolderUID   = "platform"
	FolderTitle = "Kalzone"

	PanelRequests = 1
	PanelErrors   = 2
	PanelLatency  = 3
	PanelMemory   = 4
	PanelCPU      = 5
	PanelLogs     = 6
	PanelTraffic  = 7

	PanelDatabaseRow  = 8
	PanelConnections  = 9
	PanelQueryTime    = 10
	PanelMySQLCPU     = 11
	PanelQueryLatency = 12
	PanelSlowQueries  = 13

	PanelRouteCounts = 18
	PanelRouteP95    = 19

	routePanelsY        = 16
	countInterval       = "1m"
	servicePanelsHeight = 24
)

type Service struct {
	Slug      string
	Name      string
	Databases []Database
}

type Database struct {
	Environment    string
	Schema         string
	User           string
	MaxConnections int
	SharedWith     []string
}

func DashboardUID(serviceSlug string) string {
	return "platform-" + serviceSlug
}

func Dashboard(s Service) map[string]any {
	return map[string]any{
		"uid": DashboardUID(s.Slug), "title": s.Name + " (" + s.Slug + ")", "tags": []string{"platform", s.Slug},
		"timezone": "browser", "refresh": "30s", "schemaVersion": 39,
		"time":       map[string]string{"from": "now-6h", "to": "now"},
		"templating": map[string]any{"list": []any{envVariable()}},
		"panels":     panels(s),
	}
}

func envVariable() map[string]any {
	return map[string]any{
		"name": "env", "label": "Environment", "type": "custom", "query": "dev,prod",
		"current": map[string]any{"text": "dev", "value": "dev"},
		"options": []map[string]any{{"text": "dev", "value": "dev", "selected": true}, {"text": "prod", "value": "prod", "selected": false}},
	}
}

func panels(s Service) []any {
	all := append(workloadPanels(s.Slug+"-${env}"), routePanels(s.Slug+"-${env}", routePanelsY)...)
	logsAt := servicePanelsHeight
	if len(s.Databases) > 0 {
		all = append(all, databasePanels(s, servicePanelsHeight)...)
		logsAt += databasePanelsHeight
	}
	return append(all, logs(PanelLogs, logsAt, fmt.Sprintf(`{container=~"%s-${env}-(blue|green)"}`, s.Slug)))
}

func workloadPanels(name string) []any {
	traefik := fmt.Sprintf(`service=~"%s-(blue|green)@file"`, name)
	container := fmt.Sprintf(`name=~"%s-(blue|green)"`, name)
	return []any{
		counted(timeseries(PanelRequests, "Requests", "short", 0, 0, fmt.Sprintf("sum(increase(traefik_service_requests_total{%s}[$__interval]))", traefik))),
		timeseries(PanelErrors, "5xx ratio", "percentunit", 8, 0, fmt.Sprintf(
			"sum(rate(traefik_service_requests_total{%s,code=~\"5..\"}[1m])) / sum(rate(traefik_service_requests_total{%s}[1m]))", traefik, traefik)),
		timeseries(PanelLatency, "p95 latency", "s", 16, 0, fmt.Sprintf(
			"histogram_quantile(0.95, sum by (le) (rate(traefik_service_request_duration_seconds_bucket{%s}[1m])))", traefik)),
		timeseries(PanelMemory, "Memory", "bytes", 0, 8, fmt.Sprintf("sum by (name) (container_memory_working_set_bytes{%s})", container)),
		timeseries(PanelCPU, "CPU", "percentunit", 8, 8, fmt.Sprintf("sum by (name) (rate(container_cpu_usage_seconds_total{%s}[1m]))", container)),
		stepped(timeseries(PanelTraffic, "Traffic weight", "percent", 16, 8, fmt.Sprintf(
			"max by (colour) (kalzone_traffic_weight_percent{service=\"%s\"})", name))),
	}
}

func timeseries(id int, title, unit string, x, y int, expr string) map[string]any {
	return map[string]any{
		"id": id, "type": "timeseries", "title": title,
		"datasource":  map[string]string{"type": "prometheus", "uid": "prometheus"},
		"gridPos":     map[string]int{"x": x, "y": y, "w": 8, "h": 8},
		"fieldConfig": map[string]any{"defaults": map[string]any{"unit": unit}, "overrides": []any{}},
		"targets":     []map[string]any{{"refId": "A", "expr": expr, "datasource": map[string]string{"type": "prometheus", "uid": "prometheus"}}},
	}
}

func counted(panel map[string]any) map[string]any {
	panel["interval"] = countInterval
	defaults := panel["fieldConfig"].(map[string]any)["defaults"].(map[string]any)
	defaults["decimals"] = 0
	defaults["custom"] = map[string]any{"drawStyle": "bars", "fillOpacity": 80, "stacking": map[string]string{"mode": "normal"}}
	panel["options"] = map[string]any{"legend": map[string]any{"displayMode": "list", "placement": "bottom", "calcs": []string{"sum"}}}
	return panel
}

func stepped(panel map[string]any) map[string]any {
	defaults := panel["fieldConfig"].(map[string]any)["defaults"].(map[string]any)
	defaults["min"], defaults["max"] = 0, 100
	defaults["custom"] = map[string]any{"lineInterpolation": "stepAfter"}
	panel["targets"].([]map[string]any)[0]["legendFormat"] = "{{colour}}"
	return panel
}

func logs(id, y int, expr string) map[string]any {
	return map[string]any{
		"id": id, "type": "logs", "title": "Logs",
		"datasource": map[string]string{"type": "loki", "uid": "loki"},
		"gridPos":    map[string]int{"x": 0, "y": y, "w": 24, "h": 12},
		"options":    map[string]any{"showTime": true, "wrapLogMessage": true, "sortOrder": "Descending"},
		"targets":    []map[string]any{{"refId": "A", "expr": expr, "datasource": map[string]string{"type": "loki", "uid": "loki"}}},
	}
}
