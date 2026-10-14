package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/adrianozp/platform/internal/integration"
	"github.com/adrianozp/platform/pkg/agentapi"
	"github.com/stretchr/testify/require"
)

func agentPost(s *integration.Suite, path string, body any) (int, []byte) {
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, s.Server.URL+path, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+integration.AgentToken)
	req.Header.Set("X-Agent-Name", "vps-1")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil
	}
	defer resp.Body.Close()
	answer, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, answer
}

func agentLater(s *integration.Suite, delay time.Duration, path string, body any) <-chan int {
	answered := make(chan int, 1)
	go func() {
		time.Sleep(delay)
		status, _ := agentPost(s, path, body)
		answered <- status
	}()
	return answered
}

func Test_AWaitAnswersOnceTheAgentFinishesTheDeployment(t *testing.T) {
	s, p := setupPlaces(t)
	d := deployDev(t, s, p)
	w, _ := claim(t, s)
	finished := agentLater(s, 1500*time.Millisecond, agentapi.FinishPath(w.DeploymentID), agentapi.Finish{Succeeded: true, Message: "live"})
	var waited struct {
		deploymentView
		EventsSummary *struct {
			Log string `json:"log"`
		} `json:"events_summary"`
	}

	resp := p.eve.MustJSON(t, http.MethodGet, "/api/v1/deployments/"+d.ID+"?wait=20s", nil, http.StatusOK, &waited)

	require.Equal(t, http.StatusNoContent, <-finished)
	require.Equal(t, "succeeded", waited.Status)
	require.Equal(t, "/api/v1/deployments/"+d.ID+"/events", waited.EventsSummary.Log)
	require.Empty(t, resp.Header.Get("Retry-After"))
}

func Test_AWaitThatEndsFirstAnswersAcceptedWithRetryAfter(t *testing.T) {
	s, p := setupPlaces(t)
	buildable(t, p, false)
	d := deployDev(t, s, p)
	var still deploymentView
	var building release

	resp := p.eve.MustJSON(t, http.MethodGet, "/api/v1/deployments/"+d.ID+"?wait=1s", nil, http.StatusAccepted, &still)
	requested := p.bob.MustJSON(t, http.MethodPost, "/api/v1/releases?wait=1s", map[string]string{"service": "places-api", "ref": "main"}, http.StatusAccepted, &building)

	require.Equal(t, "pending", still.Status)
	require.Equal(t, "5", resp.Header.Get("Retry-After"))
	require.Equal(t, "building", building.Status)
	require.Equal(t, "5", requested.Header.Get("Retry-After"))
}

func Test_AWaitReturnsAtOnceWhenTheServerDrains(t *testing.T) {
	s, p := setupPlaces(t)
	d := deployDev(t, s, p)
	s.Drain.Close()
	started := time.Now()

	resp := p.eve.Do(t, http.MethodGet, "/api/v1/deployments/"+d.ID+"?wait=60s", nil)

	require.Equal(t, http.StatusAccepted, resp.Status, resp.Body)
	require.Less(t, time.Since(started), 5*time.Second)
}

func Test_ABadWaitIsRefusedBeforeAnythingStarts(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	var refused apiError
	var history page[deploymentView]

	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments?wait=5m", map[string]string{"service": "places-api", "environment": "dev", "release": releaseID}, http.StatusBadRequest, &refused)
	p.bob.MustJSON(t, http.MethodGet, "/api/v1/deployments?service=places-api&env=dev", nil, http.StatusOK, &history)

	require.Equal(t, "wait", refused.Error.Field)
	require.Empty(t, history.Items)
}

func Test_AGatedCallWaitsForItsChangeRequest(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	agent := prodAgent(t, s, p)
	var asked pendingChange
	var settled requestView

	waiting := agent.MustJSON(t, http.MethodPost, "/api/v1/deployments?wait=1s", map[string]string{"service": "places-api", "environment": "prod", "release": releaseID}, http.StatusAccepted, &asked)
	approved := decide(t, p.mia, asked, "approve", url.Values{})
	agent.MustJSON(t, http.MethodGet, asked.Poll+"?wait=5s", nil, http.StatusOK, &settled)

	require.Equal(t, "change_request", asked.Kind)
	require.Equal(t, "pending", asked.Status)
	require.Equal(t, "5", waiting.Header.Get("Retry-After"))
	require.Equal(t, http.StatusNoContent, approved.Status, approved.Body)
	require.Equal(t, "executed", settled.Status)
}
