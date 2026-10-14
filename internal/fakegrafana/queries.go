package fakegrafana

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

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

func (g *Grafana) prometheus(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
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
