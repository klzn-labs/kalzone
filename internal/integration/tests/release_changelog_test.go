package tests

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/fakegithub"
	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/stretchr/testify/require"
)

type changelogBody struct {
	BaseRelease int    `json:"base_release"`
	State       string `json:"state"`
	CompareURL  string `json:"compare_url"`
	Commits     []struct {
		SHA         string `json:"sha"`
		Title       string `json:"title"`
		Type        string `json:"type"`
		PullRequest *struct {
			Number int `json:"number"`
		} `json:"pull_request"`
	} `json:"commits"`
}

type releaseWithChangelog struct {
	ID        string         `json:"id"`
	Changelog *changelogBody `json:"changelog"`
}

func sha40(c string) string {
	return strings.Repeat(c, 40)
}

func builtFrom(t *testing.T, s *integration.Suite, p people, commitSHA string) string {
	t.Helper()
	var rel release
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/releases", map[string]string{"service": "places-api", "ref": "main"}, http.StatusAccepted, &rel)
	built := callback(t, s, rel.ID, "built", repo, map[string]string{"digest": digest, "commit_sha": commitSHA, "tag": "t-" + commitSHA[:7]})
	require.Equal(t, http.StatusOK, built.Status, built.Body)
	return rel.ID
}

func settledChangelog(t *testing.T, p people, id string) changelogBody {
	t.Helper()
	var got releaseWithChangelog
	require.Eventually(t, func() bool {
		p.bob.MustJSON(t, http.MethodGet, "/api/v1/releases/"+id, nil, http.StatusOK, &got)
		return got.Changelog != nil && got.Changelog.State != "pending"
	}, 5*time.Second, 50*time.Millisecond)
	return *got.Changelog
}

func Test_ReleaseChangelog_FrozenAfterBuild(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)
	s.GitHub.WillCommit(repo, fakegithub.Commit{SHA: sha40("a"), Message: "feat: start"})
	s.GitHub.WillCompare(repo, sha40("a"), sha40("c"), fakegithub.Comparison{Status: "ahead", AheadBy: 2, Commits: []fakegithub.Commit{
		{SHA: sha40("b"), Message: "fix: one", PullNumber: 7, PullTitle: "One"},
		{SHA: sha40("c"), Message: "feat: two"},
	}})

	first := builtFrom(t, s, p, sha40("a"))
	require.Equal(t, "first", settledChangelog(t, p, first).State)
	second := builtFrom(t, s, p, sha40("c"))
	got := settledChangelog(t, p, second)

	require.Equal(t, "ahead", got.State)
	require.Equal(t, 1, got.BaseRelease)
	require.Equal(t, []string{sha40("c"), sha40("b")}, []string{got.Commits[0].SHA, got.Commits[1].SHA})
	require.Equal(t, 7, got.Commits[1].PullRequest.Number)
	require.Nil(t, got.Commits[0].PullRequest)
}

type requestPreview struct {
	Status  string `json:"status"`
	Preview *struct {
		Lines []map[string]string `json:"lines"`
		Ships *struct {
			State       string `json:"state"`
			Direction   string `json:"direction"`
			CommitCount int    `json:"commit_count"`
			Releases    []struct {
				Number  int `json:"number"`
				Commits []struct {
					SHA string `json:"sha"`
				} `json:"commits"`
			} `json:"releases"`
		} `json:"ships"`
	} `json:"preview"`
}

func Test_ProdRequest_ShowsWhatShipsBeforeAndAfterApproval(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)
	s.GitHub.WillCommit(repo, fakegithub.Commit{SHA: sha40("a"), Message: "feat: start", PullNumber: 3, PullTitle: "Start"})
	first := builtFrom(t, s, p, sha40("a"))
	settledChangelog(t, p, first)
	agent := prodAgent(t, s, p)

	pending := ask(t, agent, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "prod", "release": first})
	var before, after requestPreview
	agent.MustJSON(t, http.MethodGet, pending.Poll, nil, http.StatusOK, &before)
	decide(t, p.mia, pending, "approve", url.Values{})
	agent.MustJSON(t, http.MethodGet, pending.Poll, nil, http.StatusOK, &after)

	require.NotEmpty(t, before.Preview.Lines)
	require.Equal(t, "first", before.Preview.Ships.State)
	require.Equal(t, 1, before.Preview.Ships.Releases[0].Number)
	require.Equal(t, sha40("a"), before.Preview.Ships.Releases[0].Commits[0].SHA)
	require.Equal(t, "executed", after.Status)
	require.Empty(t, after.Preview.Lines)
	require.Equal(t, before.Preview.Ships, after.Preview.Ships)
}

func Test_ProdRequest_GitHubDownStillMatchesTheContract(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)
	s.GitHub.WillCommit(repo, fakegithub.Commit{SHA: sha40("a"), Message: "feat: start"})
	first := builtFrom(t, s, p, sha40("a"))
	settledChangelog(t, p, first)
	agent := liveOnProd(t, s, p, first)
	second := builtFrom(t, s, p, sha40("c"))
	s.GitHub.FailHistory(repo, true)

	pending := ask(t, agent, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "prod", "release": second})
	var got requestPreview
	agent.MustJSON(t, http.MethodGet, pending.Poll, nil, http.StatusOK, &got)

	require.Equal(t, "failed", got.Preview.Ships.State)
	require.Equal(t, "forward", got.Preview.Ships.Direction)
}

func liveOnProd(t *testing.T, s *integration.Suite, p people, releaseID string) *integration.Client {
	t.Helper()
	agent := prodAgent(t, s, p)
	decide(t, p.mia, ask(t, agent, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "prod", "release": releaseID}), "approve", url.Values{})
	runToSuccess(t, s)
	return agent
}

func Test_RequestPage_DivergedShowsWhatLeaves(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)
	s.GitHub.WillCommit(repo, fakegithub.Commit{SHA: sha40("a"), Message: "fix: hotfix on prod"})
	first := builtFrom(t, s, p, sha40("a"))
	settledChangelog(t, p, first)
	agent := liveOnProd(t, s, p, first)
	s.GitHub.WillCompare(repo, sha40("a"), sha40("c"), fakegithub.Comparison{Status: "diverged", AheadBy: 1, BehindBy: 1, Commits: []fakegithub.Commit{{SHA: sha40("c"), Message: "feat: main work"}}})
	s.GitHub.WillCompare(repo, sha40("c"), sha40("a"), fakegithub.Comparison{Status: "diverged", AheadBy: 1, BehindBy: 1, Commits: []fakegithub.Commit{{SHA: sha40("a"), Message: "fix: hotfix on prod"}}})
	second := builtFrom(t, s, p, sha40("c"))

	pending := ask(t, agent, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "prod", "release": second})
	resp := p.mia.Do(t, http.MethodGet, "/requests/"+pending.ID, nil)

	require.Contains(t, resp.Body, "What leaves prod")
	require.Contains(t, resp.Body, "fix: hotfix on prod")
}
