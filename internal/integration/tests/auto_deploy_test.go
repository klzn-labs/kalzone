package tests

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/stretchr/testify/require"
)

func autoDeploying(t *testing.T, p people, service string) {
	t.Helper()
	p.bob.MustJSON(t, http.MethodPatch, "/api/v1/services/"+service, map[string]any{"auto_deploy_dev": true}, http.StatusOK, nil)
}

func devDeployments(t *testing.T, c *integration.Client, service string) []deploymentView {
	t.Helper()
	var history struct {
		Items []deploymentView `json:"items"`
	}
	c.MustJSON(t, http.MethodGet, "/api/v1/deployments?service="+service+"&env=dev", nil, http.StatusOK, &history)
	return history.Items
}

func finishBuild(t *testing.T, s *integration.Suite, releaseID string) {
	t.Helper()
	built := callback(t, s, releaseID, "built", repo, map[string]string{"digest": digest, "commit_sha": commit, "tag": "r1-0123456", "run_url": runURL})
	require.Equal(t, http.StatusOK, built.Status, built.Body)
}

func Test_APushBuildDeploysItsServicesThatOptInToDev(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, true)
	addServiceOfTheSameImage(t, p)
	autoDeploying(t, p, "places-api")
	autoDeploying(t, p, "places-worker")

	webhook(t, s, "push", push("refs/heads/main"), signed(push("refs/heads/main")))
	finishBuild(t, s, releasesOf(t, p.bob, "places-api")[0].ID)
	api, worker := devDeployments(t, p.bob, "places-api"), devDeployments(t, p.bob, "places-worker")

	require.Len(t, api, 1)
	require.Equal(t, "push", api[0].Trigger)
	require.Equal(t, "pending", api[0].Status)
	require.Len(t, worker, 1)
	require.Equal(t, "push", worker[0].Trigger)
	_, claimed := claim(t, s)
	require.Equal(t, http.StatusOK, claimed)
}

func Test_AutoDeployLeavesManualBuildsAndOptedOutServicesAlone(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, true)
	addServiceOfTheSameImage(t, p)
	autoDeploying(t, p, "places-api")
	var manual release
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/releases", map[string]string{"service": "places-api", "ref": "main"}, http.StatusAccepted, &manual)

	finishBuild(t, s, manual.ID)
	webhook(t, s, "push", push("refs/heads/main"), signed(push("refs/heads/main")))
	finishBuild(t, s, releasesOf(t, p.bob, "places-api")[0].ID)

	require.Len(t, devDeployments(t, p.bob, "places-api"), 1)
	require.Empty(t, devDeployments(t, p.bob, "places-worker"))
}

func Test_AutoDeployNeedsARepositoryAndIsSetFromTheReleasesPage(t *testing.T) {
	_, p := setupPlaces(t)

	refused := p.bob.Do(t, http.MethodPatch, "/api/v1/services/places-api", map[string]any{"auto_deploy_dev": true})
	buildable(t, p, false)
	saved := p.bob.Form(t, "/services/places-api/build-settings", url.Values{"_csrf": {p.bob.CSRFToken}, "auto_deploy_dev": {"on"}, "build_workflow": {"platform-build.yml"}})
	page := p.bob.Do(t, http.MethodGet, "/services/places-api/releases", nil)
	var service struct {
		AutoDeployDev bool `json:"auto_deploy_dev"`
	}
	p.bob.MustJSON(t, http.MethodGet, "/api/v1/services/places-api", nil, http.StatusOK, &service)

	require.Equal(t, http.StatusBadRequest, refused.Status, refused.Body)
	require.Equal(t, http.StatusNoContent, saved.Status, saved.Body)
	require.True(t, service.AutoDeployDev)
	require.Contains(t, page.Body, `name="auto_deploy_dev" checked`)
}
