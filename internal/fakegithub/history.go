package fakegithub

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Commit struct {
	SHA        string
	Message    string
	Login      string
	PullNumber int
	PullTitle  string
}

type Comparison struct {
	Status   string
	AheadBy  int
	BehindBy int
	Total    int
	Commits  []Commit
}

type history struct {
	compares     map[string]Comparison
	commits      map[string]Commit
	failing      map[string]bool
	pullsDenied  map[string]bool
	compareCalls int
	pullCalls    int
}

func newHistory() history {
	return history{compares: map[string]Comparison{}, commits: map[string]Commit{}, failing: map[string]bool{}, pullsDenied: map[string]bool{}}
}

func (s *Server) routeHistory(mux *http.ServeMux) {
	mux.HandleFunc("GET /repos/{owner}/{repo}/compare/{basehead}", s.compare)
	mux.HandleFunc("GET /repos/{owner}/{repo}/commits/{sha}", s.commit)
	mux.HandleFunc("GET /repos/{owner}/{repo}/commits/{sha}/pulls", s.commitPulls)
}

func (s *Server) WillCompare(repo, base, head string, c Comparison) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.Total == 0 {
		c.Total = len(c.Commits)
	}
	s.history.compares[repo+"@"+base+"..."+head] = c
	for _, commit := range c.Commits {
		s.history.commits[repo+"@"+commit.SHA] = commit
	}
}

func (s *Server) WillCommit(repo string, c Commit) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history.commits[repo+"@"+c.SHA] = c
}

func (s *Server) FailHistory(repo string, failing bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history.failing[repo] = failing
}

func (s *Server) DenyPulls(repo string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history.pullsDenied[repo] = true
}

func (s *Server) CompareCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.history.compareCalls
}

func (s *Server) PullCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.history.pullCalls
}

func (s *Server) compare(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) || s.failed(w, r) {
		return
	}
	s.mu.Lock()
	s.history.compareCalls++
	c, ok := s.history.compares[repoName(r)+"@"+r.PathValue("basehead")]
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	base, _, _ := strings.Cut(r.PathValue("basehead"), "...")
	writeJSON(w, compareJSON(repoName(r), base, c))
}

func compareJSON(repo, base string, c Comparison) map[string]any {
	commits := make([]map[string]any, 0, len(c.Commits))
	for _, commit := range c.Commits {
		commits = append(commits, commitJSON(commit))
	}
	return map[string]any{
		"status": c.Status, "ahead_by": c.AheadBy, "behind_by": c.BehindBy, "total_commits": c.Total,
		"html_url": fmt.Sprintf("https://github.com/%s/compare/%s", repo, base), "commits": commits,
	}
}

func commitJSON(c Commit) map[string]any {
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	author := map[string]any{"login": c.Login}
	if c.Login == "" {
		author = nil
	}
	return map[string]any{
		"sha": c.SHA, "author": author,
		"commit": map[string]any{"message": c.Message, "author": map[string]any{"name": "Git " + c.Login, "date": at}},
	}
}

func (s *Server) commit(w http.ResponseWriter, r *http.Request) {
	c, ok := s.knownCommit(w, r)
	if ok {
		writeJSON(w, commitJSON(c))
	}
}

func (s *Server) commitPulls(w http.ResponseWriter, r *http.Request) {
	c, ok := s.knownCommit(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	s.history.pullCalls++
	denied := s.history.pullsDenied[repoName(r)]
	s.mu.Unlock()
	if denied {
		w.WriteHeader(http.StatusForbidden)
		writeJSON(w, map[string]string{"message": "Resource not accessible by integration"})
		return
	}
	pulls := []map[string]any{}
	if c.PullNumber != 0 {
		url := fmt.Sprintf("https://github.com/%s/pull/%d", repoName(r), c.PullNumber)
		pulls = append(pulls, map[string]any{"number": c.PullNumber, "title": c.PullTitle, "html_url": url, "merged_at": "2026-10-19T12:00:00Z"})
	}
	writeJSON(w, pulls)
}

func (s *Server) knownCommit(w http.ResponseWriter, r *http.Request) (Commit, bool) {
	if !s.authorized(w, r) || s.failed(w, r) {
		return Commit{}, false
	}
	s.mu.Lock()
	c, ok := s.history.commits[repoName(r)+"@"+r.PathValue("sha")]
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
	}
	return c, ok
}

func (s *Server) failed(w http.ResponseWriter, r *http.Request) bool {
	s.mu.Lock()
	failing := s.history.failing[repoName(r)]
	s.mu.Unlock()
	if failing {
		w.WriteHeader(http.StatusBadGateway)
	}
	return failing
}
