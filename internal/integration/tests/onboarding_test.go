package tests

import (
	"net/http"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/fakeoidc"
	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/stretchr/testify/require"
)

func credentialsFor(t *testing.T, s *integration.Suite, releaseID, repository, workflowRef string) integration.Response {
	t.Helper()
	token := s.OIDC.Mint(fakeoidc.Token{Audience: integration.Audience, Repository: repository, WorkflowRef: workflowRef, Expires: time.Minute})
	return s.Raw(t, http.MethodPost, "/api/v1/releases/"+releaseID+"/registry-credentials", nil, map[string]string{"Authorization": "Bearer " + token})
}

func Test_ABuildGetsRegistryCredentialsFromThePlatform(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)
	var rel release
	var audit auditEvents
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/releases", map[string]string{"service": "places-api", "ref": "main"}, http.StatusAccepted, &rel)
	ref := integration.BuildWorkflow + "@refs/heads/main"

	issued := credentialsFor(t, s, rel.ID, repo, ref)
	otherRepo := credentialsFor(t, s, rel.ID, "evil/repo", ref)
	ownWorkflow := credentialsFor(t, s, rel.ID, repo, repo+"/.github/workflows/steal.yml@refs/heads/main")
	p.root.MustJSON(t, http.MethodGet, "/api/v1/audit", nil, http.StatusOK, &audit)

	require.Equal(t, http.StatusOK, issued.Status, issued.Body)
	require.JSONEq(t, `{"registry":"docker.io","username":"bot","password":"`+integration.RegistryToken+`"}`, issued.Body)
	require.Equal(t, "no-store", issued.CacheControl)
	require.Equal(t, http.StatusForbidden, otherRepo.Status)
	require.Equal(t, http.StatusForbidden, ownWorkflow.Status)
	require.True(t, hasAction(audit, "registry.credentials_issued"))
}

func Test_AFinishedBuildGetsNoRegistryCredentials(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)
	var rel release
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/releases", map[string]string{"service": "places-api", "ref": "main"}, http.StatusAccepted, &rel)
	callback(t, s, rel.ID, "failed", repo, map[string]string{"failure": "boom"})

	late := credentialsFor(t, s, rel.ID, repo, integration.BuildWorkflow+"@refs/heads/main")

	require.Equal(t, http.StatusForbidden, late.Status, late.Body)
	require.NotContains(t, late.Body, integration.RegistryToken)
}

type onboardingReport struct {
	Ready  bool `json:"ready"`
	Checks []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Hint   string `json:"hint"`
	} `json:"checks"`
}

func (r onboardingReport) statuses() map[string]string {
	out := map[string]string{}
	for _, c := range r.Checks {
		out[c.Name] = c.Status
	}
	return out
}

func readyRepository(t *testing.T, s *integration.Suite, p people) {
	t.Helper()
	buildable(t, p, false)
	s.GitHub.SetFile(repo, "main", ".github/workflows/platform-build.yml", "jobs:\n  build:\n    uses: "+integration.BuildWorkflow+"@main\n")
	s.GitHub.SetFile(repo, "main", "Dockerfile", "FROM scratch")
	s.Registry.Pushable("adrianozdp/places-api")
}

func Test_OnboardingPassesWhenTheRepositoryIsReady(t *testing.T) {
	s, p := setupPlaces(t)
	readyRepository(t, s, p)
	var report onboardingReport

	p.bob.MustJSON(t, http.MethodGet, "/api/v1/services/places-api/onboarding", nil, http.StatusOK, &report)

	require.True(t, report.Ready, report.statuses())
	require.Len(t, report.Checks, 5)
}

func Test_OnboardingSaysWhatIsMissing(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)
	s.GitHub.Uninstall(repo)
	var report onboardingReport

	p.bob.MustJSON(t, http.MethodGet, "/api/v1/services/places-api/onboarding", nil, http.StatusOK, &report)

	require.False(t, report.Ready)
	require.Equal(t, map[string]string{"github_app": "failed", "workflow": "skipped", "dockerfile": "skipped", "registry": "failed", "config": "ok"}, report.statuses())
	require.Contains(t, report.Checks[0].Hint, "All repositories")
}

func Test_AnOnboardingPullRequestAddsTheWorkflow(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)
	agent := s.WithToken(t, issue(t, p.bob, tokenBody("onboarder", "read", "services")).Token)
	var opened, again struct {
		URL    string `json:"url"`
		Number int    `json:"number"`
		Branch string `json:"branch"`
	}

	agent.MustJSON(t, http.MethodPost, "/api/v1/services/places-api/onboarding/pull-request", nil, http.StatusCreated, &opened)
	agent.MustJSON(t, http.MethodPost, "/api/v1/services/places-api/onboarding/pull-request", nil, http.StatusCreated, &again)
	workflow, written := s.GitHub.File(repo, "platform-onboarding", ".github/workflows/platform-build.yml")
	refused := p.eve.Do(t, http.MethodPost, "/api/v1/services/places-api/onboarding/pull-request", nil)

	require.Equal(t, 1, opened.Number)
	require.Equal(t, opened, again)
	require.Len(t, s.GitHub.Pulls(repo), 1)
	require.True(t, written)
	require.Contains(t, workflow, "uses: "+integration.BuildWorkflow+"@main")
	require.Equal(t, http.StatusNotFound, refused.Status, refused.Body)
}

func Test_AServiceWithTheWorkflowGetsNoOnboardingPullRequest(t *testing.T) {
	s, p := setupPlaces(t)
	readyRepository(t, s, p)

	resp := p.bob.Do(t, http.MethodPost, "/api/v1/services/places-api/onboarding/pull-request", nil)

	require.Equal(t, http.StatusConflict, resp.Status, resp.Body)
}

func Test_TheOnboardingToolsFollowTheTokenScopes(t *testing.T) {
	s, p := setupPlaces(t)

	reader := listTools(t, mcpSession(t, s, issue(t, p.bob, tokenBody("reader", "read")).Token))
	editor := listTools(t, mcpSession(t, s, issue(t, p.bob, tokenBody("editor", "read", "services")).Token))

	require.Contains(t, reader, "getOnboarding")
	require.NotContains(t, reader, "createOnboardingPullRequest")
	require.Contains(t, editor, "createOnboardingPullRequest")
}
