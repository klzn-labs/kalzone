package fakegithub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
)

type User struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

type Server struct {
	*httptest.Server
	mu           sync.Mutex
	byCode       map[string]User
	dispatches   []Dispatch
	tokensIssued int
	repos        map[string]*repository
	uninstalled  map[string]bool
	history      history
}

func New() *Server {
	s := &Server{byCode: map[string]User{}, repos: map[string]*repository{}, uninstalled: map[string]bool{}, history: newHistory()}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/oauth/access_token", s.token)
	mux.HandleFunc("GET /user", s.user)
	s.routeApp(mux)
	s.routeRepos(mux)
	s.routeHistory(mux)
	s.Server = httptest.NewServer(mux)
	return s
}

func (s *Server) WillSignIn(code string, user User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byCode[code] = user
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	_, known := s.byCode[r.FormValue("code")]
	s.mu.Unlock()
	if !known || r.FormValue("client_secret") == "" {
		writeJSON(w, map[string]string{"error": "bad_verification_code"})
		return
	}
	writeJSON(w, map[string]string{"access_token": "token:" + r.FormValue("code")})
}

func (s *Server) user(w http.ResponseWriter, r *http.Request) {
	code := r.Header.Get("Authorization")[len("Bearer token:"):]
	s.mu.Lock()
	user, known := s.byCode[code]
	s.mu.Unlock()
	if !known {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	writeJSON(w, user)
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}
