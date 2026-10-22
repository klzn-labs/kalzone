package entities

type walk struct {
	start, spread, min, max float64
}

var demoWalks = map[string]walk{
	"short":       {start: 40, spread: 6, min: 0, max: 120},
	"percentunit": {start: 0.01, spread: 0.003, min: 0, max: 0.05},
	"s":           {start: 0.12, spread: 0.01, min: 0.03, max: 0.4},
	"bytes":       {start: 200e6, spread: 2e6, min: 150e6, max: 260e6},
	"percent":     {start: 100, spread: 0, min: 100, max: 100},
}

var testDataSource = map[string]string{"type": "grafana-testdata-datasource", "uid": TestDataUID}

func DemoDashboard(s Service) map[string]any {
	d := Dashboard(s)
	for _, panel := range d["panels"].([]any) {
		simulate(panel.(map[string]any))
	}
	return d
}

func simulate(panel map[string]any) {
	if _, queried := panel["targets"]; !queried {
		return
	}
	panel["datasource"] = testDataSource
	if panel["type"] == "logs" {
		panel["targets"] = []map[string]any{{"refId": "A", "datasource": testDataSource, "scenarioId": "logs"}}
		return
	}
	w := demoWalks[unitOf(panel)]
	panel["targets"] = []map[string]any{{
		"refId": "A", "datasource": testDataSource, "scenarioId": "random_walk", "seriesCount": 1,
		"startValue": w.start, "spread": w.spread, "min": w.min, "max": w.max,
	}}
}

func unitOf(panel map[string]any) string {
	config, _ := panel["fieldConfig"].(map[string]any)
	defaults, _ := config["defaults"].(map[string]any)
	unit, _ := defaults["unit"].(string)
	return unit
}
