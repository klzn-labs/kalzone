package tests

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/adrianozp/platform/internal/fakeoidc"
	"github.com/adrianozp/platform/internal/integration"
	"github.com/stretchr/testify/require"
)

const (
	repo   = "adrianozp/go-plateful"
	digest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	commit = "0123456789abcdef0123456789abcdef01234567"
	runURL = "https://github.com/adrianozp/go-plateful/actions/runs/9"
)

type release struct {
	ID        string `json:"id"`
	Number    int    `json:"number"`
	Status    string `json:"status"`
	Reference string `json:"reference"`
	RunURL    string `json:"run_url"`
	Failure   string `json:"failure"`
}

func buildable(t *testing.T, p people, autoBuild bool) {
	t.Helper()
	p.bob.MustJSON(t, http.MethodPatch, "/api/v1/services/places-api", map[string]any{"github_repo": repo, "auto_build": autoBuild}, http.StatusOK, nil)
}

func ciToken(s *integration.Suite, repository string) map[string]string {
	token := s.OIDC.Mint(fakeoidc.Token{Audience: integration.Audience, Repository: repository, Expires: time.Minute})
	return map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/json"}
}

func callback(t *testing.T, s *integration.Suite, id, step, repository string, body any) integration.Response {
	t.Helper()
	raw, _ := json.Marshal(body)
	return s.Raw(t, http.MethodPost, "/api/v1/releases/"+id+"/"+step, raw, ciToken(s, repository))
}

func Test_ABuildGoesFromRequestToBuilt(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)
	var rel release

	p.bob.MustJSON(t, http.MethodPost, "/api/v1/releases", map[string]string{"service": "places-api", "ref": "main"}, http.StatusAccepted, &rel)
	started := callback(t, s, rel.ID, "started", repo, map[string]string{"run_url": runURL})
	built := callback(t, s, rel.ID, "built", repo, map[string]string{"digest": digest, "commit_sha": commit, "tag": "r1-0123456", "run_url": runURL})

	require.Equal(t, "building", rel.Status)
	dispatch := s.GitHub.Dispatches()[0]
	require.Equal(t, repo, dispatch.Repository)
	require.Equal(t, "platform-build.yml", dispatch.Workflow)
	require.Equal(t, "main", dispatch.Ref)
	require.Equal(t, rel.ID, dispatch.Inputs["release_id"])
	require.Equal(t, http.StatusOK, started.Status, started.Body)
	require.Equal(t, http.StatusOK, built.Status, built.Body)
	var stored release
	p.eve.MustJSON(t, http.MethodGet, "/api/v1/releases/"+rel.ID, nil, http.StatusOK, &stored)
	require.Equal(t, "built", stored.Status)
	require.Equal(t, "docker.io/adrianozdp/places-api@"+digest, stored.Reference)
	require.Equal(t, runURL, stored.RunURL)
}

func Test_BuildCallbacksNeedATokenFromTheServicesRepository(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)
	var rel release
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/releases", map[string]string{"service": "places-api", "ref": "main"}, http.StatusAccepted, &rel)

	anonymous := s.Raw(t, http.MethodPost, "/api/v1/releases/"+rel.ID+"/built", []byte(`{"digest":"`+digest+`"}`), nil)
	wrongRepo := callback(t, s, rel.ID, "built", "evil/fork", map[string]string{"digest": digest})
	wrongAudience := s.Raw(t, http.MethodPost, "/api/v1/releases/"+rel.ID+"/built", []byte(`{"digest":"`+digest+`"}`),
		map[string]string{"Authorization": "Bearer " + s.OIDC.Mint(fakeoidc.Token{Audience: "https://other", Repository: repo, Expires: time.Minute})})

	require.Equal(t, http.StatusUnauthorized, anonymous.Status)
	require.Equal(t, http.StatusForbidden, wrongRepo.Status)
	require.Equal(t, http.StatusUnauthorized, wrongAudience.Status)
}

func Test_AFailedBuildIsRecorded(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)
	var rel release
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/releases", map[string]string{"service": "places-api", "ref": "main"}, http.StatusAccepted, &rel)

	resp := callback(t, s, rel.ID, "failed", repo, map[string]string{"failure": "go test failed", "run_url": runURL})

	require.Equal(t, http.StatusOK, resp.Status, resp.Body)
	require.Contains(t, resp.Body, `"status":"failed"`)
	require.Contains(t, resp.Body, "go test failed")
}

func signed(body []byte) string {
	mac := hmac.New(sha256.New, []byte(integration.WebhookSecret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func push(ref string) []byte {
	raw, _ := json.Marshal(map[string]any{
		"ref": ref, "after": commit,
		"repository": map[string]string{"full_name": repo, "default_branch": "main"},
	})
	return raw
}

func webhook(t *testing.T, s *integration.Suite, event string, body []byte, signature string) integration.Response {
	t.Helper()
	return s.Raw(t, http.MethodPost, "/webhooks/github", body, map[string]string{"X-GitHub-Event": event, "X-Hub-Signature-256": signature})
}

func Test_APushToMainBuildsServicesWithAutoBuild(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, true)

	unsigned := webhook(t, s, "push", push("refs/heads/main"), "sha256=00")
	feature := webhook(t, s, "push", push("refs/heads/feature"), signed(push("refs/heads/feature")))
	main := webhook(t, s, "push", push("refs/heads/main"), signed(push("refs/heads/main")))

	require.Equal(t, http.StatusUnauthorized, unsigned.Status)
	require.Equal(t, http.StatusNoContent, feature.Status)
	require.Equal(t, http.StatusNoContent, main.Status)
	require.Len(t, s.GitHub.Dispatches(), 1)
	require.Equal(t, commit, s.GitHub.Dispatches()[0].Ref)
}

func Test_AWorkflowRunThatFailsFailsTheRelease(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)
	var rel release
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/releases", map[string]string{"service": "places-api", "ref": "main"}, http.StatusAccepted, &rel)
	body, _ := json.Marshal(map[string]any{
		"action": "completed", "repository": map[string]string{"full_name": repo},
		"workflow_run": map[string]string{"conclusion": "failure", "display_title": "platform release " + rel.ID, "html_url": runURL},
	})

	resp := webhook(t, s, "workflow_run", body, signed(body))

	require.Equal(t, http.StatusNoContent, resp.Status)
	var stored release
	p.bob.MustJSON(t, http.MethodGet, "/api/v1/releases/"+rel.ID, nil, http.StatusOK, &stored)
	require.Equal(t, "failed", stored.Status)
	require.Equal(t, "workflow run failure", stored.Failure)
}

func Test_RegisteringATagPinsItsDigest(t *testing.T) {
	s, p := setupPlaces(t)
	s.Registry.Has("adrianozdp/places-api", "1.4.0", digest)
	var rel release

	p.bob.MustJSON(t, http.MethodPost, "/api/v1/releases/registrations", map[string]string{"service": "places-api", "tag": "1.4.0"}, http.StatusCreated, &rel)
	missing := p.bob.Do(t, http.MethodPost, "/api/v1/releases/registrations", map[string]string{"service": "places-api", "tag": "9.9.9"})
	outsider := p.eve.Do(t, http.MethodPost, "/api/v1/releases/registrations", map[string]string{"service": "places-api", "tag": "1.4.0"})

	require.Equal(t, "built", rel.Status)
	require.Equal(t, "docker.io/adrianozdp/places-api@"+digest, rel.Reference)
	require.Equal(t, http.StatusNotFound, missing.Status)
	require.Equal(t, http.StatusForbidden, outsider.Status)
}

func Test_TheReleasesPageBuildsAndSavesSettings(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)

	settings := p.bob.Form(t, "/services/places-api/build-settings", url.Values{"_csrf": {p.bob.CSRFToken}, "auto_build": {"on"}, "build_workflow": {"build.yml"}})
	build := p.bob.Form(t, "/services/places-api/releases", url.Values{"_csrf": {p.bob.CSRFToken}, "ref": {"main"}})
	page := p.bob.Do(t, http.MethodGet, "/services/places-api/releases", nil)

	require.Equal(t, http.StatusNoContent, settings.Status, settings.Body)
	require.Equal(t, http.StatusNoContent, build.Status, build.Body)
	require.Equal(t, "build.yml", s.GitHub.Dispatches()[0].Workflow)
	require.Contains(t, page.Body, "r1")
	require.Contains(t, page.Body, "building")
	require.Contains(t, page.Body, "on every push")
}
