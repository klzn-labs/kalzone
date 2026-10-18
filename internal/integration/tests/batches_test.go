package tests

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/klzn-labs/kalzone/pkg/agentapi"
	"github.com/stretchr/testify/require"
)

var (
	devTargets  = []map[string]string{{"service": "places-api", "environment": "dev"}, {"service": "places-worker", "environment": "dev"}}
	prodTargets = []map[string]string{{"service": "places-api", "environment": "prod"}, {"service": "places-worker", "environment": "prod"}}
)

func sharedBuild(t *testing.T, s *integration.Suite, p people) {
	t.Helper()
	buildable(t, p, false)
	addServiceOfTheSameImage(t, p)
	var rel release
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/releases", map[string]string{"service": "places-api", "ref": "main"}, http.StatusAccepted, &rel)
	built := callback(t, s, rel.ID, "built", repo, map[string]string{"digest": digest, "commit_sha": commit, "tag": "r1-0123456", "run_url": runURL})
	require.Equal(t, http.StatusOK, built.Status, built.Body)
}

func agentRunsLater(s *integration.Suite, count int) <-chan []int {
	finished := make(chan []int, 1)
	go func() {
		time.Sleep(time.Second)
		finished <- runEach(s, count)
	}()
	return finished
}

func runEach(s *integration.Suite, count int) []int {
	statuses := []int{}
	for range count {
		statuses = append(statuses, claimAndFinish(s))
	}
	return statuses
}

func claimAndFinish(s *integration.Suite) int {
	status, raw := agentPost(s, agentapi.ClaimPath, nil)
	var w agentapi.Workload
	if status != http.StatusOK || json.Unmarshal(raw, &w) != nil {
		return status
	}
	finished, _ := agentPost(s, agentapi.FinishPath(w.DeploymentID), agentapi.Finish{Succeeded: true, Message: "live"})
	return finished
}

func Test_ABatchDeploysOneSharedBuildEverywhereAndWaitsForIt(t *testing.T) {
	s, p := setupPlaces(t)
	sharedBuild(t, s, p)
	finished := agentRunsLater(s, 2)
	var done page[deploymentView]

	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments?wait=60s", map[string]any{"targets": devTargets, "release": "sha:0123456"}, http.StatusOK, &done)

	require.Equal(t, []int{http.StatusNoContent, http.StatusNoContent}, <-finished)
	require.Len(t, done.Items, 2)
	require.Equal(t, []string{"succeeded", "succeeded"}, []string{done.Items[0].Status, done.Items[1].Status})
	require.Equal(t, []int{1, 1}, []int{done.Items[0].ReleaseNumber, done.Items[1].ReleaseNumber})
}

func Test_ABatchIsAllOrNothing(t *testing.T) {
	s, p := setupPlaces(t)
	sharedBuild(t, s, p)
	targets := append(devTargets, map[string]string{"service": "places-api", "environment": "qa"})
	var refused apiError
	var history page[deploymentView]

	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]any{"targets": targets, "release": "latest"}, http.StatusNotFound, &refused)
	p.bob.MustJSON(t, http.MethodGet, "/api/v1/deployments?service=places-api&env=dev", nil, http.StatusOK, &history)

	require.Equal(t, "targets[2]", refused.Error.Field)
	require.Len(t, refused.Error.Details["targets"], 1)
	require.Empty(t, history.Items)
}

func Test_ABatchToProtectedTargetsNeedsABypass(t *testing.T) {
	s, p := setupPlaces(t)
	sharedBuild(t, s, p)
	bypass := agentFor(t, s, p.root, map[string]any{"name": "root-bypass", "scopes": []string{"deploy", "protected"}, "teams": []string{"places"}, "bypass": true})
	var refused apiError
	var history, created page[deploymentView]

	prodAgent(t, s, p).MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]any{"targets": prodTargets, "release": "latest"}, http.StatusForbidden, &refused)
	p.mia.MustJSON(t, http.MethodGet, "/api/v1/deployments?service=places-api&env=prod", nil, http.StatusOK, &history)
	bypass.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]any{"targets": prodTargets, "release": "latest"}, http.StatusAccepted, &created)

	require.Equal(t, "err_approval_required", refused.Error.Code)
	require.Equal(t, "targets[0]", refused.Error.Field)
	require.Contains(t, refused.Error.Hint, "one at a time")
	require.Empty(t, history.Items)
	require.Len(t, created.Items, 2)
}

func Test_ABatchPromotesWhatRunsInTheSourceEnvironment(t *testing.T) {
	s, p := setupPlaces(t)
	sharedBuild(t, s, p)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]any{"targets": devTargets, "release": "latest"}, http.StatusAccepted, nil)
	require.Equal(t, []int{http.StatusNoContent, http.StatusNoContent}, runEach(s, 2))
	var promoted page[deploymentView]

	p.mia.MustJSON(t, http.MethodPost, "/api/v1/deployments/promotions", map[string]any{"targets": prodTargets, "from": "dev"}, http.StatusAccepted, &promoted)

	require.Len(t, promoted.Items, 2)
	require.Equal(t, "promote", promoted.Items[1].Trigger)
	require.Equal(t, 1, promoted.Items[1].ReleaseNumber)
}

func Test_ADeployTakesOneFormOfTarget(t *testing.T) {
	_, p := setupPlaces(t)
	var both, partial apiError

	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]any{"service": "places-api", "targets": devTargets, "release": "latest"}, http.StatusBadRequest, &both)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments/promotions", map[string]any{"service": "places-api", "from": "dev"}, http.StatusBadRequest, &partial)

	require.Equal(t, "targets", both.Error.Field)
	require.Equal(t, "to", partial.Error.Field)
}

func Test_TheDocumentStatesABatchsLimits(t *testing.T) {
	_, p := setupPlaces(t)
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]map[string]any `json:"properties"`
				Required   []string                  `json:"required"`
			} `json:"schemas"`
		} `json:"components"`
	}

	p.eve.MustJSON(t, http.MethodGet, "/api/v1/openapi.json", nil, http.StatusOK, &doc)

	for _, body := range []string{"DeploymentsDeploy", "DeploymentsPromote"} {
		targets := doc.Components.Schemas[body].Properties["targets"]
		require.Equal(t, []any{float64(1), float64(10)}, []any{targets["minItems"], targets["maxItems"]}, body)
	}
	require.ElementsMatch(t, []string{"service", "environment"}, doc.Components.Schemas["DeploymentsTarget"].Required)
}
