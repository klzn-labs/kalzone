package tests

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/klzn-labs/kalzone/internal/fakegithub"
	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/stretchr/testify/require"
)

func twoReleasesWithHistory(t *testing.T, s *integration.Suite, p people) {
	t.Helper()
	buildable(t, p, false)
	s.GitHub.WillCommit(repo, fakegithub.Commit{SHA: sha40("a"), Message: "feat: start"})
	s.GitHub.WillCompare(repo, sha40("a"), sha40("c"), fakegithub.Comparison{Status: "ahead", AheadBy: 2, Commits: []fakegithub.Commit{
		{SHA: sha40("b"), Message: "fix: one", PullNumber: 7, PullTitle: "Fix the first thing"},
		{SHA: sha40("c"), Message: "feat: two"},
	}})
	settledChangelog(t, p, builtFrom(t, s, p, sha40("a")))
	settledChangelog(t, p, builtFrom(t, s, p, sha40("c")))
}

func Test_ReleasePage_ShowsChangelog(t *testing.T) {
	s, p := setupPlaces(t)
	twoReleasesWithHistory(t, s, p)

	resp := p.mia.Do(t, http.MethodGet, "/services/places-api/releases/2", nil)

	require.Equal(t, http.StatusOK, resp.Status, resp.Body)
	require.Contains(t, resp.Body, "Changelog since r1")
	require.Contains(t, resp.Body, "Fix the first thing")
	require.Contains(t, resp.Body, "#7")
	require.Contains(t, resp.Body, "feat: two")
}

func Test_ReleasePage_UnknownNumberIs404(t *testing.T) {
	_, p := setupPlaces(t)

	resp := p.mia.Do(t, http.MethodGet, "/services/places-api/releases/42", nil)

	require.Equal(t, http.StatusNotFound, resp.Status)
}

func Test_ReleasesList_LinksToReleasePage(t *testing.T) {
	s, p := setupPlaces(t)
	twoReleasesWithHistory(t, s, p)

	resp := p.mia.Do(t, http.MethodGet, "/services/places-api/releases", nil)

	require.Contains(t, resp.Body, `href="/services/places-api/releases/2"`)
	require.Contains(t, resp.Body, "2 commits since r1")
	require.Contains(t, resp.Body, "first release")
}

func Test_RequestPage_ShowsWhatShipsAfterApproval(t *testing.T) {
	s, p := setupPlaces(t)
	twoReleasesWithHistory(t, s, p)
	var r2 release
	p.mia.MustJSON(t, http.MethodGet, "/api/v1/releases/r2?service=places-api", nil, http.StatusOK, &r2)
	pending := ask(t, prodAgent(t, s, p), http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "prod", "release": r2.ID})
	decide(t, p.mia, pending, "approve", url.Values{})

	resp := p.mia.Do(t, http.MethodGet, "/requests/"+pending.ID, nil)

	require.Contains(t, resp.Body, "What ships to prod")
	require.Contains(t, resp.Body, "Fix the first thing")
	require.Contains(t, resp.Body, sha40("c")[:7])
}
