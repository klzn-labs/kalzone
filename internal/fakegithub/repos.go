package fakegithub

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type PullRequest struct {
	Number int
	Title  string
	Head   string
	Base   string
	Body   string
}

type repository struct {
	files map[string]map[string]string
	pulls []PullRequest
}

func (s *Server) routeRepos(mux *http.ServeMux) {
	mux.HandleFunc("GET /repos/{owner}/{repo}", s.repo)
	mux.HandleFunc("GET /repos/{owner}/{repo}/contents/{path...}", s.getFile)
	mux.HandleFunc("PUT /repos/{owner}/{repo}/contents/{path...}", s.putFile)
	mux.HandleFunc("GET /repos/{owner}/{repo}/git/ref/heads/{branch...}", s.getRef)
	mux.HandleFunc("POST /repos/{owner}/{repo}/git/refs", s.createRef)
	mux.HandleFunc("GET /repos/{owner}/{repo}/pulls", s.listPulls)
	mux.HandleFunc("POST /repos/{owner}/{repo}/pulls", s.createPull)
}

func (s *Server) Uninstall(repo string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uninstalled[repo] = true
}

func (s *Server) SetFile(repo, branch, path, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.repoLocked(repo).files[branch][path] = content
}

func (s *Server) File(repo, branch, path string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, ok := s.repoLocked(repo).files[branch][path]
	return content, ok
}

func (s *Server) Pulls(repo string) []PullRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]PullRequest(nil), s.repoLocked(repo).pulls...)
}

func (s *Server) repoLocked(name string) *repository {
	r, ok := s.repos[name]
	if !ok {
		r = &repository{files: map[string]map[string]string{"main": {}}}
		s.repos[name] = r
	}
	return r
}

func repoName(r *http.Request) string {
	return r.PathValue("owner") + "/" + r.PathValue("repo")
}

func (s *Server) authorized(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Authorization") == "token ghs_installation" {
		return true
	}
	w.WriteHeader(http.StatusUnauthorized)
	return false
}

func (s *Server) repo(w http.ResponseWriter, r *http.Request) {
	if s.authorized(w, r) {
		writeJSON(w, map[string]string{"default_branch": "main", "full_name": repoName(r)})
	}
}

func (s *Server) getFile(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	branch := firstOf(r.URL.Query().Get("ref"), "main")
	content, ok := s.File(repoName(r), branch, r.PathValue("path"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, map[string]string{"content": base64.StdEncoding.EncodeToString([]byte(content)), "encoding": "base64", "sha": "sha-" + r.PathValue("path")})
}

func (s *Server) putFile(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	var body struct {
		Content string `json:"content"`
		Branch  string `json:"branch"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	raw, _ := base64.StdEncoding.DecodeString(body.Content)
	s.SetFile(repoName(r), body.Branch, r.PathValue("path"), string(raw))
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"content": map[string]string{"path": r.PathValue("path")}})
}

func (s *Server) getRef(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	s.mu.Lock()
	_, ok := s.repoLocked(repoName(r)).files[r.PathValue("branch")]
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, map[string]any{"object": map[string]string{"sha": "sha-" + r.PathValue("branch")}})
}

func (s *Server) createRef(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	var body struct {
		Ref string `json:"ref"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	branch := strings.TrimPrefix(body.Ref, "refs/heads/")
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked(repoName(r))
	if _, exists := repo.files[branch]; exists {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	repo.files[branch] = copyFiles(repo.files["main"])
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{"ref": body.Ref})
}

func copyFiles(files map[string]string) map[string]string {
	copied := map[string]string{}
	for k, v := range files {
		copied[k] = v
	}
	return copied
}

func (s *Server) listPulls(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	head := r.URL.Query().Get("head")
	open := []map[string]any{}
	for _, p := range s.Pulls(repoName(r)) {
		if head == "" || strings.HasSuffix(head, ":"+p.Head) {
			open = append(open, pullJSON(repoName(r), p))
		}
	}
	writeJSON(w, open)
}

func (s *Server) createPull(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	var p PullRequest
	_ = json.NewDecoder(r.Body).Decode(&struct {
		Title *string `json:"title"`
		Head  *string `json:"head"`
		Base  *string `json:"base"`
		Body  *string `json:"body"`
	}{&p.Title, &p.Head, &p.Base, &p.Body})
	s.mu.Lock()
	repo := s.repoLocked(repoName(r))
	p.Number = len(repo.pulls) + 1
	repo.pulls = append(repo.pulls, p)
	s.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, pullJSON(repoName(r), p))
}

func pullJSON(repo string, p PullRequest) map[string]any {
	return map[string]any{"number": p.Number, "html_url": fmt.Sprintf("https://github.com/%s/pull/%d", repo, p.Number), "head": map[string]string{"ref": p.Head}}
}

func firstOf(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
