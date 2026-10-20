package entities

import (
	"fmt"
	"sort"
	"strings"
)

const (
	databasePanelsHeight = 19
	mysqlContainer       = "plateful-mysql-1"
	statements           = "mysql_perf_schema_events_statements"
	sharedCPU            = "Shared by every service on this MySQL server"
)

func DatabaseRowTitle(dbs []Database) string {
	seen := map[string]bool{}
	for _, db := range dbs {
		for _, slug := range db.SharedWith {
			seen[slug] = true
		}
	}
	if len(seen) == 0 {
		return "Database"
	}
	others := make([]string, 0, len(seen))
	for slug := range seen {
		others = append(others, slug)
	}
	sort.Strings(others)
	return "Database (shared with " + strings.Join(others, ", ") + ")"
}

func databasePanels(s Service, y int) []any {
	attached := fmt.Sprintf(`kalzone_database_attachment{service="%s",environment="$env"}`, s.Slug)
	return []any{
		row(PanelDatabaseRow, DatabaseRowTitle(s.Databases), y),
		connections(attached, y+1),
		timeseries(PanelQueryTime, "Query time", "s", 8, y+1, perSchema(fmt.Sprintf("rate(%s_seconds_total[$__rate_interval])", statements), attached)),
		mysqlCPU(y + 1),
		sized(timeseries(PanelQueryLatency, "Average query time", "s", 0, y+9, averageLatency(attached)), 8, 10),
		slowQueries(attached, y+9),
	}
}

func perSchema(series, attached string) string {
	return fmt.Sprintf("sum(%s and on(schema) %s)", series, attached)
}

func averageLatency(attached string) string {
	return perSchema(fmt.Sprintf("rate(%s_seconds_total[$__rate_interval])", statements), attached) + " / " +
		perSchema(fmt.Sprintf("rate(%s_total[$__rate_interval])", statements), attached)
}

func row(id int, title string, y int) map[string]any {
	return map[string]any{
		"id": id, "type": "row", "title": title, "collapsed": false, "panels": []any{},
		"gridPos": map[string]int{"x": 0, "y": y, "w": 24, "h": 1},
	}
}

func sized(panel map[string]any, w, h int) map[string]any {
	grid := panel["gridPos"].(map[string]int)
	grid["w"], grid["h"] = w, h
	return panel
}

func connections(attached string, y int) map[string]any {
	panel := timeseries(PanelConnections, "Connections", "none", 0, y,
		fmt.Sprintf("sum(mysql_info_schema_processlist_processes_by_user and on(mysql_user) %s)", attached))
	targets := panel["targets"].([]map[string]any)
	targets[0]["legendFormat"] = "open"
	limit := prometheusTarget("B", fmt.Sprintf("max(%s)", attached))
	limit["legendFormat"] = "limit (max_connections)"
	panel["targets"] = append(targets, limit)
	panel["fieldConfig"].(map[string]any)["overrides"] = []any{limitOverride("limit (max_connections)")}
	return panel
}

func limitOverride(name string) map[string]any {
	return map[string]any{
		"matcher": map[string]string{"id": "byName", "options": name},
		"properties": []map[string]any{
			{"id": "color", "value": map[string]string{"mode": "fixed", "fixedColor": "red"}},
			{"id": "custom.lineStyle", "value": map[string]any{"fill": "dash", "dash": []int{10, 10}}},
		},
	}
}

func mysqlCPU(y int) map[string]any {
	panel := timeseries(PanelMySQLCPU, "MySQL CPU (shared)", "percentunit", 16, y,
		fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{name="%s"}[$__rate_interval]))`, mysqlContainer))
	panel["description"] = sharedCPU
	return panel
}

func prometheusTarget(refID, expr string) map[string]any {
	return map[string]any{"refId": refID, "expr": expr, "datasource": map[string]string{"type": "prometheus", "uid": "prometheus"}}
}
