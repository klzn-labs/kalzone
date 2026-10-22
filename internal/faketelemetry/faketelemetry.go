package faketelemetry

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
)

type Received struct {
	Method  string
	Path    string
	Form    url.Values
	Tenants string
	Auth    string
}

type Store struct {
	*httptest.Server
	mu       sync.Mutex
	received []Received
}

func New() *Store {
	s := &Store{}
	s.Server = httptest.NewServer(http.HandlerFunc(s.answer))
	return s
}

func (s *Store) Received() []Received {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Received{}, s.received...)
}

func (s *Store) answer(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	_, _ = io.Copy(io.Discard, r.Body)
	s.mu.Lock()
	s.received = append(s.received, Received{
		Method: r.Method, Path: r.URL.Path, Form: r.Form, Tenants: r.Header.Get("X-Scope-OrgID"), Auth: r.Header.Get("Authorization"),
	})
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
}
