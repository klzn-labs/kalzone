package entities

import "fmt"

const (
	callsField    = "Value #A"
	totalField    = "Value #B"
	examinedField = "Value #C"
	avgField      = "Avg time"
	rowsField     = "Rows examined / call"
)

func slowQueries(attached string, y int) map[string]any {
	return map[string]any{
		"id": PanelSlowQueries, "type": "table", "title": "Slowest queries",
		"description": "Statements of this database in the selected time range, slowest average first",
		"datasource":  map[string]string{"type": "prometheus", "uid": "prometheus"},
		"gridPos":     map[string]int{"x": 8, "y": y, "w": 16, "h": 10},
		"targets": []map[string]any{
			rangeTotal("A", "total", attached), rangeTotal("B", "seconds_total", attached), rangeTotal("C", "rows_examined_total", attached),
		},
		"transformations": []any{
			map[string]any{"id": "merge", "options": map[string]any{}},
			ratio(totalField, callsField, avgField), ratio(examinedField, callsField, rowsField),
			organize(),
			map[string]any{"id": "sortBy", "options": map[string]any{"sort": []map[string]any{{"field": avgField, "desc": true}}}},
		},
		"fieldConfig": map[string]any{"defaults": map[string]any{}, "overrides": []any{unit(avgField, "s"), unit("Total time", "s"), unit(rowsField, "short")}},
	}
}

func rangeTotal(refID, counter, attached string) map[string]any {
	target := prometheusTarget(refID, fmt.Sprintf("sum by (digest_text) (increase(%s_%s[$__range]) and on(schema) %s)", statements, counter, attached))
	target["instant"], target["range"], target["format"] = true, false, "table"
	return target
}

func ratio(left, right, alias string) map[string]any {
	return map[string]any{"id": "calculateField", "options": map[string]any{
		"mode": "binary", "alias": alias,
		"binary": map[string]any{
			"left":     map[string]any{"matcher": map[string]string{"id": "byName", "options": left}},
			"operator": "/",
			"right":    map[string]any{"matcher": map[string]string{"id": "byName", "options": right}},
		},
	}}
}

func organize() map[string]any {
	return map[string]any{"id": "organize", "options": map[string]any{
		"excludeByName": map[string]bool{"Time": true, examinedField: true},
		"renameByName":  map[string]string{"digest_text": "Query", callsField: "Calls", totalField: "Total time"},
		"indexByName":   map[string]int{"digest_text": 0, callsField: 1, avgField: 2, totalField: 3, rowsField: 4},
	}}
}

func unit(field, u string) map[string]any {
	return map[string]any{
		"matcher":    map[string]string{"id": "byName", "options": field},
		"properties": []map[string]any{{"id": "unit", "value": u}},
	}
}
