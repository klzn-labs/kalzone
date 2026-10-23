package tests

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/klzn-labs/kalzone/internal/mcpserver"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
	"github.com/stretchr/testify/require"
)

const cronPath = "/api/v1/crons/dev/nightly-import"

type cronRunDTO struct {
	ID     string `json:"id"`
	Manual bool   `json:"manual"`
	Status string `json:"status"`
}

type cronDTO struct {
	Name        string      `json:"name"`
	Environment string      `json:"environment"`
	Service     string      `json:"service"`
	Schedule    string      `json:"schedule"`
	Path        string      `json:"path"`
	Payload     any         `json:"payload"`
	Timeout     string      `json:"timeout"`
	Paused      bool        `json:"paused"`
	NextRunAt   *time.Time  `json:"next_run_at"`
	LastRun     *cronRunDTO `json:"last_run"`
}

type cronPage struct {
	Items []cronDTO `json:"items"`
}

type cronRunPage struct {
	Items []cronRunDTO `json:"items"`
}

type desiredCrons struct {
	Crons    []queuedapi.Cron    `json:"crons"`
	Commands []queuedapi.Command `json:"commands"`
}

func cronBody(env string) map[string]any {
	return map[string]any{"name": "nightly-import", "environment": env, "service": "places-api", "schedule": "0 3 * * *", "path": "/hooks/import"}
}

func createCron(t *testing.T, c *integration.Client, env string) cronDTO {
	t.Helper()
	var created cronDTO
	c.MustJSON(t, http.MethodPost, "/api/v1/crons", cronBody(env), http.StatusCreated, &created)
	return created
}

func gatewayCrons(t *testing.T, s *integration.Suite) desiredCrons {
	t.Helper()
	resp := s.Queued(t, http.MethodGet, queuedapi.DesiredPath, nil)
	require.Equal(t, http.StatusOK, resp.Status, resp.Body)
	var desired desiredCrons
	decode(t, resp.Body, &desired)
	return desired
}

func Test_CronsAreCreatedWithDefaultsAndEditedByPolicy(t *testing.T) {
	_, p := setupPlaces(t)

	created := createCron(t, p.bob, "dev")
	bobProd := p.bob.Do(t, http.MethodPost, "/api/v1/crons", cronBody("prod"))
	eve := p.eve.Do(t, http.MethodPatch, cronPath, map[string]any{"schedule": "@hourly"})
	var updated cronDTO
	p.bob.MustJSON(t, http.MethodPatch, cronPath, map[string]any{"schedule": "@hourly", "payload": map[string]bool{"full": true}, "timeout": "5m"}, http.StatusOK, &updated)
	invalid := p.bob.Do(t, http.MethodPatch, cronPath, map[string]any{"schedule": "@every 1m"})

	require.Equal(t, "places-api", created.Service)
	require.Equal(t, "1m0s", created.Timeout)
	require.Equal(t, map[string]any{}, created.Payload)
	require.NotNil(t, created.NextRunAt)
	require.Equal(t, http.StatusForbidden, bobProd.Status, bobProd.Body)
	require.Equal(t, http.StatusForbidden, eve.Status, eve.Body)
	require.Equal(t, "@hourly", updated.Schedule)
	require.Equal(t, "5m0s", updated.Timeout)
	require.Equal(t, map[string]any{"full": true}, updated.Payload)
	require.Equal(t, http.StatusBadRequest, invalid.Status, invalid.Body)
}

func Test_ACronIsListedPausedResumedAndDeleted(t *testing.T) {
	_, p := setupPlaces(t)
	createCron(t, p.bob, "dev")
	var list cronPage
	var paused, resumed cronDTO

	p.bob.MustJSON(t, http.MethodGet, "/api/v1/crons?environment=dev&team=places", nil, http.StatusOK, &list)
	p.bob.MustJSON(t, http.MethodPost, cronPath+"/pause", nil, http.StatusOK, &paused)
	p.bob.MustJSON(t, http.MethodPost, cronPath+"/resume", nil, http.StatusOK, &resumed)
	deleted := p.bob.Do(t, http.MethodDelete, cronPath, nil)
	gone := p.bob.Do(t, http.MethodGet, cronPath, nil)

	require.Len(t, list.Items, 1)
	require.True(t, paused.Paused)
	require.Nil(t, paused.NextRunAt)
	require.False(t, resumed.Paused)
	require.NotNil(t, resumed.NextRunAt)
	require.Equal(t, http.StatusNoContent, deleted.Status, deleted.Body)
	require.Equal(t, http.StatusNotFound, gone.Status, gone.Body)
}

func Test_ACronInProdNeedsApproval(t *testing.T) {
	s, p := setupPlaces(t)
	agent := prodAgent(t, s, p)

	pending := ask(t, agent, http.MethodPost, "/api/v1/crons", cronBody("prod"))
	before := p.mia.Do(t, http.MethodGet, "/api/v1/crons/prod/nightly-import", nil)
	approved := decide(t, p.root, pending, "approve", url.Values{})
	var c cronDTO
	p.mia.MustJSON(t, http.MethodGet, "/api/v1/crons/prod/nightly-import", nil, http.StatusOK, &c)
	pause := agent.Do(t, http.MethodPost, "/api/v1/crons/prod/nightly-import/pause", nil)

	require.Equal(t, http.StatusNotFound, before.Status, before.Body)
	require.Equal(t, http.StatusNoContent, approved.Status, approved.Body)
	require.Equal(t, "executed", poll(t, agent, pending).Status)
	require.Equal(t, "0 3 * * *", c.Schedule)
	require.Equal(t, http.StatusForbidden, pause.Status, pause.Body)
}

func Test_ResourcesListCronsWithTheirService(t *testing.T) {
	_, p := setupPlaces(t)
	createCron(t, p.bob, "dev")
	var crons resourcePage

	p.bob.MustJSON(t, http.MethodGet, "/api/v1/resources?kind=cron", nil, http.StatusOK, &crons)

	require.Equal(t, []resourceDTO{{Kind: "cron", Name: "nightly-import", Environment: "dev", Attached: []string{"places-api"}}}, crons.Items)
}

func Test_TheGatewaySignsCronsWithTheServicesQueueKey(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	createCron(t, p.bob, "dev")
	deployPlaces(t, p.bob, releaseID)
	w := runToSuccess(t, s)

	desired := gatewayCrons(t, s)

	require.Len(t, desired.Crons, 1)
	c := desired.Crons[0]
	require.Equal(t, "http://places-api-dev:8080/hooks/import", c.Target)
	require.Equal(t, w.Secrets["PLATFORM_QUEUE_SIGNING_KEY"], c.SigningKey)
	require.Equal(t, "0 3 * * *", c.Schedule)
	require.Equal(t, "1m0s", c.Timeout)
	require.JSONEq(t, "{}", string(c.Payload))
}

func Test_RunNowReachesTheGatewayUntilDone(t *testing.T) {
	s, p := setupPlaces(t)
	createCron(t, p.bob, "dev")
	var requested struct {
		RunID string `json:"run_id"`
	}

	p.bob.MustJSON(t, http.MethodPost, cronPath+"/runs", nil, http.StatusAccepted, &requested)
	desired := gatewayCrons(t, s)
	done := s.Queued(t, http.MethodPost, queuedapi.DonePath(requested.RunID), nil)
	after := gatewayCrons(t, s)

	require.Equal(t, []queuedapi.Command{{ID: requested.RunID, CronID: desired.Crons[0].ID, Kind: "run"}}, desired.Commands)
	require.Equal(t, http.StatusNoContent, done.Status, done.Body)
	require.Empty(t, after.Commands)
}

func reportRuns(t *testing.T, s *integration.Suite, cronID string, count int) {
	t.Helper()
	at := time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)
	runs := []queuedapi.CronRun{}
	for i := range count {
		scheduled := at.Add(time.Duration(i) * time.Minute)
		runs = append(runs, queuedapi.CronRun{
			ID: fmt.Sprintf("run-%02d", i), CronID: cronID, Status: "succeeded", ScheduledAt: scheduled,
			StartedAt: scheduled, FinishedAt: scheduled.Add(time.Second), Attempts: 1, LastStatus: http.StatusOK,
		})
	}
	resp := s.Queued(t, http.MethodPost, queuedapi.CronRunsPath, queuedapi.CronRunReport{Runs: append(runs, runs[count-1])})
	require.Equal(t, http.StatusNoContent, resp.Status, resp.Body)
}

func Test_ReportedRunsAreDedupedAndTheNewest50Kept(t *testing.T) {
	s, p := setupPlaces(t)
	createCron(t, p.bob, "dev")
	cronID := gatewayCrons(t, s).Crons[0].ID
	var runs cronRunPage
	var c cronDTO

	reportRuns(t, s, cronID, 55)
	p.bob.MustJSON(t, http.MethodGet, cronPath+"/runs?limit=100", nil, http.StatusOK, &runs)
	p.bob.MustJSON(t, http.MethodGet, cronPath, nil, http.StatusOK, &c)

	require.Len(t, runs.Items, 50)
	require.Equal(t, "run-54", runs.Items[0].ID)
	require.Equal(t, "run-05", runs.Items[49].ID)
	require.Equal(t, &cronRunDTO{ID: "run-54", Status: "succeeded"}, c.LastRun)
}

func Test_DeletingAServiceDeletesItsCrons(t *testing.T) {
	s, p := setupPlaces(t)
	createCron(t, p.bob, "dev")

	view := startDeleting(t, p.mia, "places-api")
	var list cronPage
	p.bob.MustJSON(t, http.MethodGet, "/api/v1/crons", nil, http.StatusOK, &list)

	require.True(t, view.Deleted)
	require.Empty(t, list.Items)
	require.Empty(t, gatewayCrons(t, s).Crons)
}

func Test_McpListsCronTools(t *testing.T) {
	s, p := setupPlaces(t)

	tools := listTools(t, mcpSession(t, s, issue(t, p.bob, tokenBody("scheduler", "read", "services")).Token))

	for _, name := range []string{"listCrons", "createCron", "getCron", "updateCron", "deleteCron", "pauseCron", "resumeCron", "runCron", "listCronRuns"} {
		require.Contains(t, tools, name)
	}
	require.False(t, mcpserver.Excluded("deleteCron"))
}
