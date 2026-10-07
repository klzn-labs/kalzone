package fakeregistry

import (
	"net/http"
	"net/http/httptest"
	"sync"
)

type Registry struct {
	*httptest.Server
	mu      sync.Mutex
	digests map[string]string
}

func New() *Registry {
	r := &Registry{digests: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("HEAD /v2/{owner}/{name}/manifests/{tag}", r.manifest)
	r.Server = httptest.NewServer(mux)
	return r
}

func (r *Registry) Has(name, tag, digest string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.digests[name+":"+tag] = digest
}

func (r *Registry) manifest(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	digest, ok := r.digests[req.PathValue("owner")+"/"+req.PathValue("name")+":"+req.PathValue("tag")]
	r.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Docker-Content-Digest", digest)
}
