package tests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/klzn-labs/kalzone/pkg/agentapi"
	"github.com/stretchr/testify/require"
)

type deploymentView struct {
	ID             string `json:"id"`
	Number         int    `json:"number"`
	Status         string `json:"status"`
	ConfigRevision int    `json:"config_revision"`
	ReleaseNumber  int    `json:"release_number"`
	Trigger        string `json:"trigger"`
	Message        string `json:"message"`
}

func builtRelease(t *testing.T, s *integration.Suite, p people) string {
	t.Helper()
	s.Registry.Has("adrianozdp/places-api", "1.0.0", digest)
	var rel release
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/releases/registrations", map[string]string{"service": "places-api", "tag": "1.0.0"}, http.StatusCreated, &rel)
	return rel.ID
}

func claim(t *testing.T, s *integration.Suite) (agentapi.Workload, int) {
	t.Helper()
	resp := s.Agent(t, http.MethodPost, agentapi.ClaimPath, nil)
	var w agentapi.Workload
	if resp.Status == http.StatusOK {
		require.NoError(t, json.Unmarshal([]byte(resp.Body), &w))
	}
	return w, resp.Status
}

func runToSuccess(t *testing.T, s *integration.Suite) agentapi.Workload {
	t.Helper()
	w, status := claim(t, s)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, http.StatusNoContent, s.Agent(t, http.MethodPost, agentapi.EventsPath(w.DeploymentID), agentapi.Events{Lines: []string{"pulling", "live"}}).Status)
	require.Equal(t, http.StatusNoContent, s.Agent(t, http.MethodPost, agentapi.FinishPath(w.DeploymentID), agentapi.Finish{Succeeded: true, Message: "live"}).Status)
	return w
}

func Test_ADeployIsClaimedRunAndBecomesLive(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	p.bob.MustJSON(t, http.MethodPut, "/api/v1/config/places-api/dev/secrets/DB_PASSWORD", map[string]string{"value": secretValue}, http.StatusOK, nil)
	var d deploymentView

	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "dev", "release": releaseID}, http.StatusAccepted, &d)
	w := runToSuccess(t, s)

	require.Equal(t, "pending", d.Status)
	require.Equal(t, 1, d.ConfigRevision)
	requirePlacesDevWorkload(t, w)
	requireSucceededWithoutSecrets(t, p, d.ID)
}

func requirePlacesDevWorkload(t *testing.T, w agentapi.Workload) {
	t.Helper()
	require.Equal(t, "places-api-dev", w.Name)
	require.Equal(t, "r1.c1", w.Version)
	require.Equal(t, "docker.io/adrianozdp/places-api@"+digest, w.ImageRef)
	require.Equal(t, secretValue, w.Secrets["DB_PASSWORD"])
	require.Equal(t, "http://queued:8080", w.Env["PLATFORM_QUEUE_URL"])
	require.Len(t, w.Secrets["PLATFORM_QUEUE_TOKEN"], 64)
	require.Len(t, w.Secrets["PLATFORM_QUEUE_SIGNING_KEY"], 64)
}

func requireSucceededWithoutSecrets(t *testing.T, p people, deploymentID string) {
	t.Helper()
	var history struct {
		Deployments []deploymentView `json:"items"`
	}
	list := p.bob.MustJSON(t, http.MethodGet, "/api/v1/deployments?service=places-api&env=dev", nil, http.StatusOK, &history)
	require.Equal(t, "succeeded", history.Deployments[0].Status)
	require.NotContains(t, list.Body, secretValue)
	detail := p.bob.Do(t, http.MethodGet, "/api/v1/deployments/"+deploymentID, nil)
	require.Contains(t, detail.Body, `"message":"pulling"`)
}

func Test_OnlyMaintainersPromoteToProd(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "dev", "release": releaseID}, http.StatusAccepted, nil)
	runToSuccess(t, s)
	promote := map[string]string{"service": "places-api", "from": "dev", "to": "prod"}

	bob := p.bob.Do(t, http.MethodPost, "/api/v1/deployments/promotions", promote)
	var d deploymentView
	p.mia.MustJSON(t, http.MethodPost, "/api/v1/deployments/promotions", promote, http.StatusAccepted, &d)

	require.Equal(t, http.StatusForbidden, bob.Status)
	require.Equal(t, "promote", d.Trigger)
	require.Equal(t, 1, d.ReleaseNumber)
}

func Test_DriftReappliesTheDesiredState(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "dev", "release": releaseID}, http.StatusAccepted, nil)
	runToSuccess(t, s)
	var desired agentapi.DesiredList
	require.NoError(t, json.Unmarshal([]byte(s.Agent(t, http.MethodGet, agentapi.DesiredPath, nil).Body), &desired))

	drift := s.Agent(t, http.MethodPost, agentapi.DriftPath, agentapi.Drift{Service: "places-api", Environment: "dev", Observed: "places-api-dev is not running"})
	w, status := claim(t, s)

	require.Len(t, desired.Workloads, 1)
	require.Equal(t, "places-api-dev", desired.Workloads[0].Name)
	require.Contains(t, drift.Body, `"reconciling":true`)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, 2, w.Number)
}

func Test_AgentsMustAuthenticate(t *testing.T) {
	s, _ := setupPlaces(t)

	noToken := s.Raw(t, http.MethodPost, agentapi.ClaimPath, nil, map[string]string{"X-Agent-Name": "vps-1"})
	badName := s.Raw(t, http.MethodPost, agentapi.ClaimPath, nil, map[string]string{"Authorization": "Bearer " + integration.AgentToken, "X-Agent-Name": "Bad Name!"})
	_, nothing := claim(t, s)

	require.Equal(t, http.StatusUnauthorized, noToken.Status)
	require.Equal(t, http.StatusBadRequest, badName.Status)
	require.Equal(t, http.StatusNoContent, nothing)
}

func Test_TheEnvironmentPageDeploysPromotesAndRollsBack(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	dev := "/services/places-api/env/dev"

	queued := p.bob.Form(t, dev+"/deployments", url.Values{"_csrf": {p.bob.CSRFToken}, "release_id": {releaseID}, "strategy": {"bluegreen"}})
	running := p.bob.Do(t, http.MethodGet, dev, nil)
	runToSuccess(t, s)
	live := p.bob.Do(t, http.MethodGet, dev, nil)
	prodPage := p.mia.Do(t, http.MethodGet, "/services/places-api/env/prod", nil)
	promoted := p.mia.Form(t, "/services/places-api/env/prod/promotions", url.Values{"_csrf": {p.mia.CSRFToken}, "from": {"dev"}})
	agentsPage := p.root.Do(t, http.MethodGet, "/agents", nil)

	require.Equal(t, http.StatusNoContent, queued.Status, queued.Body)
	require.Contains(t, running.Body, "Waiting for an agent")
	require.Contains(t, live.Body, "Live in Development")
	require.Contains(t, live.Body, "deploy-panel")
	require.Contains(t, prodPage.Body, "Promote r1 from dev")
	require.Equal(t, http.StatusNoContent, promoted.Status, promoted.Body)
	require.Contains(t, agentsPage.Body, "vps-1")
}

func Test_ProdOffersNoPromotionOfTheReleaseItAlreadyRuns(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	prod := "/services/places-api/env/prod"
	p.bob.Form(t, "/services/places-api/env/dev/deployments", url.Values{"_csrf": {p.bob.CSRFToken}, "release_id": {releaseID}, "strategy": {"bluegreen"}})
	runToSuccess(t, s)
	p.mia.Form(t, prod+"/promotions", url.Values{"_csrf": {p.mia.CSRFToken}, "from": {"dev"}})
	runToSuccess(t, s)

	prodPage := p.mia.Do(t, http.MethodGet, prod, nil)

	require.Contains(t, prodPage.Body, "Live in Production")
	require.NotContains(t, prodPage.Body, "Promote r1 from dev")
}
