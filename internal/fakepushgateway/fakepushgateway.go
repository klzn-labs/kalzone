package fakepushgateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

type Pushgateway struct {
	*httptest.Server
	mu     sync.Mutex
	groups map[string]string
}

func New() *Pushgateway {
	p := &Pushgateway{groups: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /metrics/job/{job}", p.put)
	mux.HandleFunc("DELETE /metrics/job/{job}", p.delete)
	p.Server = httptest.NewServer(mux)
	return p
}

func (p *Pushgateway) Body(job string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.groups[job]
}

func (p *Pushgateway) put(w http.ResponseWriter, r *http.Request) {
	var body strings.Builder
	_, _ = io.Copy(&body, r.Body)
	p.mu.Lock()
	p.groups[r.PathValue("job")] = body.String()
	p.mu.Unlock()
}

func (p *Pushgateway) delete(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	delete(p.groups, r.PathValue("job"))
	p.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
}
