package fakegrafana

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"

	"encoding/json"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
)

const (
	AdminUser     = "admin"
	AdminPassword = "grafana-pass"
)

type Grafana struct {
	*httptest.Server
	mu          sync.Mutex
	folders     map[int]bool
	dashboards  map[int]map[string]string
	orgs        map[string]int
	members     map[int]map[string]string
	users       map[string]int
	datasources map[int]map[string]map[string]any
	usage       map[string]usage
	series      map[string][]float64
	lines       []LokiLine
	lokiQueries []string
}

func New() *Grafana {
	g := &Grafana{
		folders: map[int]bool{}, dashboards: map[int]map[string]string{1: {}}, orgs: map[string]int{"Main Org.": 1},
		members: map[int]map[string]string{1: {AdminUser: "Admin"}}, users: map[string]int{AdminUser: 1},
		datasources: map[int]map[string]map[string]any{}, usage: map[string]usage{}, series: map[string][]float64{},
	}
	mux := http.NewServeMux()
	g.routeDashboards(mux)
	g.routeOrgs(mux)
	mux.HandleFunc("GET /grafana/api/datasources/proxy/uid/prometheus/api/v1/query", g.prometheus)
	mux.HandleFunc("GET /grafana/api/datasources/proxy/uid/prometheus/api/v1/query_range", g.prometheusRange)
	mux.HandleFunc("GET /grafana/api/datasources/proxy/uid/loki/loki/api/v1/query_range", g.loki)
	mux.HandleFunc("/grafana/", echo)
	g.Server = httptest.NewServer(mux)
	return g
}

func (g *Grafana) routeDashboards(mux *http.ServeMux) {
	mux.HandleFunc("GET /grafana/api/folders/platform", g.admin(g.getFolder))
	mux.HandleFunc("POST /grafana/api/folders", g.admin(g.createFolder))
	mux.HandleFunc("POST /grafana/api/dashboards/db", g.admin(g.dashboard))
	mux.HandleFunc("DELETE /grafana/api/dashboards/uid/{uid}", g.admin(g.deleteDashboard))
	mux.HandleFunc("GET /grafana/api/search", g.admin(g.search))
}

func (g *Grafana) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != AdminUser || password != AdminPassword {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func orgOf(r *http.Request) int {
	org, err := strconv.Atoi(r.Header.Get("X-Grafana-Org-Id"))
	if err != nil {
		return 1
	}
	return org
}

func (g *Grafana) Dashboards() map[string]bool {
	return g.OrgDashboards(1)
}

func (g *Grafana) OrgDashboards(org int) map[string]bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	copied := map[string]bool{}
	for k := range g.dashboards[org] {
		copied[k] = true
	}
	return copied
}

func (g *Grafana) DashboardJSON(uid string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.dashboards[1][uid]
}

func (g *Grafana) getFolder(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.folders[orgOf(r)] {
		w.WriteHeader(http.StatusNotFound)
	}
}

func (g *Grafana) createFolder(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	g.folders[orgOf(r)] = true
	g.mu.Unlock()
}

func (g *Grafana) dashboard(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dashboard map[string]any `json:"dashboard"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	uid, _ := body.Dashboard["uid"].(string)
	raw, _ := json.Marshal(body.Dashboard)
	g.mu.Lock()
	g.dashboards[orgOf(r)][uid] = string(raw)
	g.mu.Unlock()
}

func (g *Grafana) deleteDashboard(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	delete(g.dashboards[orgOf(r)], r.PathValue("uid"))
	g.mu.Unlock()
}

func (g *Grafana) search(w http.ResponseWriter, r *http.Request) {
	found := []map[string]string{}
	for uid := range g.OrgDashboards(orgOf(r)) {
		found = append(found, map[string]string{"uid": uid})
	}
	_ = json.NewEncoder(w).Encode(found)
}

func echo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"path": r.URL.Path, "user": assertedUser(r.Header.Get("X-JWT-Assertion")), "proxy_user": r.Header.Get("X-WEBAUTH-USER"),
	})
}

func assertedUser(assertion string) string {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(assertion, claims); err != nil {
		return ""
	}
	sub, _ := claims["sub"].(string)
	return sub
}

func SigningKey() string {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}
