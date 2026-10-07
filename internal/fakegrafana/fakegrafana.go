package fakegrafana

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
)

type Grafana struct {
	*httptest.Server
	mu         sync.Mutex
	folder     bool
	dashboards map[string]bool
}

func New() *Grafana {
	g := &Grafana{dashboards: map[string]bool{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /grafana/api/folders/platform", g.getFolder)
	mux.HandleFunc("POST /grafana/api/folders", g.createFolder)
	mux.HandleFunc("POST /grafana/api/dashboards/db", g.dashboard)
	mux.HandleFunc("/grafana/", echo)
	g.Server = httptest.NewServer(mux)
	return g
}

func (g *Grafana) Dashboards() map[string]bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	copied := map[string]bool{}
	for k, v := range g.dashboards {
		copied[k] = v
	}
	return copied
}

func (g *Grafana) getFolder(w http.ResponseWriter, _ *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.folder {
		w.WriteHeader(http.StatusNotFound)
	}
}

func (g *Grafana) createFolder(w http.ResponseWriter, _ *http.Request) {
	g.mu.Lock()
	g.folder = true
	g.mu.Unlock()
}

func (g *Grafana) dashboard(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dashboard map[string]any `json:"dashboard"`
	}
	if r.Header.Get("Authorization") != "Bearer grafana-token" || json.NewDecoder(r.Body).Decode(&body) != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	uid, _ := body.Dashboard["uid"].(string)
	g.mu.Lock()
	g.dashboards[uid] = true
	g.mu.Unlock()
}

func echo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"path": r.URL.Path, "user": r.Header.Get("X-WEBAUTH-USER"), "role": r.Header.Get("X-WEBAUTH-ROLE"),
	})
}
