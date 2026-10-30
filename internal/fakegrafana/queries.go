package fakegrafana

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const sustainedSamples = 5

var containerName = regexp.MustCompile(`name="([^"]+)"`)

type LokiLine struct {
	Container string
	At        time.Time
	Line      string
}

type usage struct {
	cpu, memory float64
}

func (g *Grafana) SetUsage(container string, cpuMillis, memoryMB float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.usage[container] = usage{cpu: cpuMillis, memory: memoryMB}
}

func (g *Grafana) SetLines(lines ...LokiLine) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lines = lines
}

func (g *Grafana) LokiQueries() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string{}, g.lokiQueries...)
}

func (g *Grafana) SetSeries(match string, values ...float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.series[match] = values
}

func (g *Grafana) seriesFor(query string) ([]float64, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for match, values := range g.series {
		if strings.Contains(query, match) && len(values) > 0 {
			return values, true
		}
	}
	return nil, false
}

func (g *Grafana) prometheusRange(w http.ResponseWriter, r *http.Request) {
	result := []any{}
	if values, ok := g.seriesFor(r.URL.Query().Get("query")); ok {
		result = append(result, map[string]any{"metric": map[string]string{}, "values": spread(values, r.URL.Query().Get("end"), r.URL.Query().Get("step"))})
	}
	writeJSON(w, map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": result}})
}

func writeVector(w http.ResponseWriter, value string) {
	vector := []any{map[string]any{"metric": map[string]string{}, "value": []any{time.Now().Unix(), value}}}
	writeJSON(w, map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": vector}})
}

func spread(values []float64, end, step string) [][2]any {
	last, _ := strconv.ParseInt(end, 10, 64)
	every, _ := strconv.ParseInt(step, 10, 64)
	points := make([][2]any, 0, len(values))
	for i, v := range values {
		at := last - int64(len(values)-1-i)*every
		points = append(points, [2]any{at, strconv.FormatFloat(v, 'f', -1, 64)})
	}
	return points
}

func instantOf(query string, values []float64) float64 {
	if !strings.HasPrefix(query, "min_over_time(") {
		return values[len(values)-1]
	}
	if len(values) < sustainedSamples {
		return 0
	}
	return slices.Min(values[len(values)-sustainedSamples:])
}

func (g *Grafana) prometheus(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if values, ok := g.seriesFor(query); ok {
		writeVector(w, strconv.FormatFloat(instantOf(query, values), 'f', -1, 64))
		return
	}
	match := containerName.FindStringSubmatch(query)
	g.mu.Lock()
	found, known := usage{}, false
	if match != nil {
		found, known = g.usage[match[1]]
	}
	g.mu.Unlock()
	result := []any{}
	if known {
		result = append(result, map[string]any{"metric": map[string]string{}, "value": []any{time.Now().Unix(), sample(query, found)}})
	}
	writeJSON(w, map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": result}})
}

func sample(query string, u usage) string {
	if strings.Contains(query, "cpu") {
		return strconv.FormatFloat(u.cpu, 'f', -1, 64)
	}
	return strconv.FormatFloat(u.memory, 'f', -1, 64)
}

func (g *Grafana) loki(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	g.lokiQueries = append(g.lokiQueries, r.URL.Query().Get("query"))
	lines := append([]LokiLine{}, g.lines...)
	g.mu.Unlock()
	result := []any{}
	for _, line := range lines {
		values := [][2]string{{strconv.FormatInt(line.At.UnixNano(), 10), line.Line}}
		result = append(result, map[string]any{"stream": map[string]string{"container": line.Container}, "values": values})
	}
	writeJSON(w, map[string]any{"status": "success", "data": map[string]any{"resultType": "streams", "result": result}})
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}
