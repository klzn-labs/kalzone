package fakegithub

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type Dispatch struct {
	Repository string
	Workflow   string
	Ref        string
	Inputs     map[string]string
}

func (s *Server) routeApp(mux *http.ServeMux) {
	mux.HandleFunc("GET /repos/{owner}/{repo}/installation", s.installation)
	mux.HandleFunc("POST /app/installations/{id}/access_tokens", s.accessToken)
	mux.HandleFunc("POST /repos/{owner}/{repo}/actions/workflows/{workflow}/dispatches", s.dispatch)
}

func (s *Server) Dispatches() []Dispatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Dispatch(nil), s.dispatches...)
}

func (s *Server) TokensIssued() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokensIssued
}

func (s *Server) installation(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]int64{"id": 77})
}

func (s *Server) accessToken(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.tokensIssued++
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs_installation", "expires_at": time.Now().Add(time.Hour)})
}

func (s *Server) dispatch(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "token ghs_installation" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var body struct {
		Ref    string            `json:"ref"`
		Inputs map[string]string `json:"inputs"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	repository := r.PathValue("owner") + "/" + r.PathValue("repo")
	s.mu.Lock()
	s.dispatches = append(s.dispatches, Dispatch{Repository: repository, Workflow: r.PathValue("workflow"), Ref: body.Ref, Inputs: body.Inputs})
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
