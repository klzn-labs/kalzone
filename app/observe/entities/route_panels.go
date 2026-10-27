package entities

import "fmt"

const (
	traefikContainer = "plateful-traefik-1"
	routeLabel       = "label_format route=\"{{if or (eq .DownstreamStatus `404`) (eq .DownstreamStatus `405`)}}unmatched{{else}}{{.RequestMethod}} {{regexReplaceAll `/[0-9]+\\\\b` (regexReplaceAll `/[0-9a-fA-F-]{16,}\\\\b` (regexReplaceAll `/[0-9]{4}-[0-9]{2}-[0-9]{2}\\\\b` .RequestPath `/:date`) `/:id`) `/:id`}}{{end}}\""
	topRoutes        = 8
	latencyInterval  = "5m"
)

func AccessLog(name string) string {
	return fmt.Sprintf(`{container="%s"} |= "RequestPath" | json | __error__="" | ServiceName=~"%s-(blue|green)@file" | %s`,
		traefikContainer, name, routeLabel)
}

func routePanels(name string, y int) []any {
	log := AccessLog(name)
	counts := counted(lokiSeries(PanelRouteCounts, "Requests by route", "short", 0, y, fmt.Sprintf(
		"topk(%d, sum by (route) (count_over_time(%s [$__interval])))", topRoutes, log)))
	p95 := lokiSeries(PanelRouteP95, "p95 by route", "s", 12, y, fmt.Sprintf(
		"topk(%d, quantile_over_time(0.95, %s | unwrap Duration [$__interval]) by (route)) / 1e9", topRoutes, log))
	p95["interval"] = latencyInterval
	return []any{legend(counts, "{{route}}"), legend(p95, "{{route}}")}
}

func lokiSeries(id int, title, unit string, x, y int, expr string) map[string]any {
	panel := timeseries(id, title, unit, x, y, expr)
	loki := map[string]string{"type": "loki", "uid": "loki"}
	panel["datasource"] = loki
	panel["targets"].([]map[string]any)[0]["datasource"] = loki
	panel["gridPos"] = map[string]int{"x": x, "y": y, "w": 12, "h": 8}
	return panel
}
