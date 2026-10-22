package tests

import (
	"context"
	"io"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/agent"
	"github.com/klzn-labs/kalzone/internal/fakegrafana"
	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/klzn-labs/kalzone/pkg/agentapi"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/stretchr/testify/require"
)

type runtimeView struct {
	Container string   `json:"container"`
	Found     bool     `json:"found"`
	State     string   `json:"state"`
	Health    string   `json:"health"`
	HealthLog []string `json:"health_log"`
	Restarts  int      `json:"restarts"`
	Stale     bool     `json:"stale"`
	Usage     *struct {
		CPUMillis      int `json:"cpu_millis"`
		CPULimitMillis int `json:"cpu_limit_millis"`
		MemoryMB       int `json:"memory_mb"`
		MemoryLimitMB  int `json:"memory_limit_mb"`
	} `json:"usage"`
}

type failureView struct {
	Status  string `json:"status"`
	Failure *struct {
		Class     string   `json:"class"`
		Hint      string   `json:"hint"`
		Container string   `json:"container"`
		HealthLog []string `json:"health_log"`
		Logs      []string `json:"logs"`
	} `json:"failure"`
}

func liveContainer() agentapi.Container {
	return agentapi.Container{
		Found: true, Name: "places-api-dev-blue", Image: "img@sha256:1", State: "running", Health: "healthy",
		HealthLog: []string{"ok"}, Restarts: 2, StartedAt: time.Now().Add(-time.Hour), MemoryLimitMB: 256, CPULimitMillis: 500,
	}
}

func reportStatus(t *testing.T, s *integration.Suite, snapshots ...agentapi.Snapshot) {
	t.Helper()
	resp := s.Agent(t, http.MethodPost, agentapi.StatusPath, agentapi.StatusReport{Workloads: snapshots})
	require.Equal(t, http.StatusNoContent, resp.Status, resp.Body)
}

func Test_TheAgentsSnapshotIsTheServicesRuntimeStatus(t *testing.T) {
	s, p := setupPlaces(t)
	s.Grafana.SetUsage("places-api-dev-blue", 3.4, 41.6)
	reportStatus(t, s,
		agentapi.Snapshot{Service: "places-api", Environment: "dev", Container: liveContainer()},
		agentapi.Snapshot{Service: "unknown", Environment: "dev", Container: agentapi.Container{}},
	)
	var view runtimeView

	p.bob.MustJSON(t, http.MethodGet, "/api/v1/runtime/places-api/dev", nil, http.StatusOK, &view)

	require.Equal(t, "places-api-dev-blue", view.Container)
	require.Equal(t, "healthy", view.Health)
	require.Equal(t, 2, view.Restarts)
	require.False(t, view.Stale)
	require.Equal(t, 3, view.Usage.CPUMillis)
	require.Equal(t, 500, view.Usage.CPULimitMillis)
	require.Equal(t, 42, view.Usage.MemoryMB)
	require.Equal(t, 256, view.Usage.MemoryLimitMB)
}

func Test_ARuntimeStatusGoesStaleWhenTheAgentStopsReporting(t *testing.T) {
	s, p := setupPlaces(t)
	clock.WithTime(time.Now().Add(-2 * time.Minute))
	reportStatus(t, s, agentapi.Snapshot{Service: "places-api", Environment: "dev", Container: liveContainer()})
	clock.Clock = nil
	var view runtimeView

	p.bob.MustJSON(t, http.MethodGet, "/api/v1/runtime/places-api/dev", nil, http.StatusOK, &view)
	missing := p.bob.Do(t, http.MethodGet, "/api/v1/runtime/places-api/prod", nil)

	require.True(t, view.Stale)
	require.Nil(t, view.Usage)
	require.Equal(t, http.StatusNotFound, missing.Status)
	require.Contains(t, missing.Body, "the agent reports every 15s")
}

func Test_AFailedDeployShowsItsClassEvidenceAndHint(t *testing.T) {
	s, p := setupPlaces(t)
	d := deployDev(t, s, p)
	w, _ := claim(t, s)
	failure := &agentapi.Failure{Class: "unhealthy", Container: "places-api-dev-green", HealthLog: []string{"HTTP 404"}, Logs: []string{"listening on :8080"}}
	var view failureView

	finish := s.Agent(t, http.MethodPost, agentapi.FinishPath(w.DeploymentID), agentapi.Finish{Succeeded: false, Message: "deployctl: exit status 10", Failure: failure})
	p.bob.MustJSON(t, http.MethodGet, "/api/v1/deployments/"+d.ID, nil, http.StatusOK, &view)

	require.Equal(t, http.StatusNoContent, finish.Status, finish.Body)
	require.Equal(t, "failed", view.Status)
	require.Equal(t, "unhealthy", view.Failure.Class)
	require.Contains(t, view.Failure.Hint, "health_path")
	require.Equal(t, []string{"HTTP 404"}, view.Failure.HealthLog)
	require.Equal(t, []string{"listening on :8080"}, view.Failure.Logs)
}

func Test_LogsComeFromLokiOldestFirst(t *testing.T) {
	s, p := setupPlaces(t)
	now := time.Now().UTC()
	s.Grafana.SetLines(
		fakegrafana.LokiLine{Container: "places-api-dev-blue", At: now.Add(-time.Second), Line: "second"},
		fakegrafana.LokiLine{Container: "places-api-dev-green", At: now.Add(-2 * time.Second), Line: "first"},
	)
	var logs struct {
		Items []struct {
			Container string `json:"container"`
			Line      string `json:"line"`
		} `json:"items"`
	}

	p.bob.MustJSON(t, http.MethodGet, "/api/v1/logs/places-api/dev?contains=boom&limit=20", nil, http.StatusOK, &logs)

	require.Equal(t, "first", logs.Items[0].Line)
	require.Equal(t, "places-api-dev-blue", logs.Items[1].Container)
	require.Equal(t, `{container=~"places-api-dev-(blue|green)"} |= "boom"`, s.Grafana.LokiQueries()[0])
}

func Test_LogsNeedTheLogsScopeAndTheOwningTeam(t *testing.T) {
	s, p := setupPlaces(t)
	path := "/api/v1/logs/places-api/dev"

	reader := s.WithToken(t, issue(t, p.bob, tokenBody("reader", "read")).Token).Do(t, http.MethodGet, path, nil)
	logsOnly := s.WithToken(t, issue(t, p.bob, tokenBody("logs-only", "logs")).Token).Do(t, http.MethodGet, path, nil)
	agent := s.WithToken(t, issue(t, p.bob, tokenBody("claude", "read", "logs")).Token).Do(t, http.MethodGet, path, nil)
	outsider := p.eve.Do(t, http.MethodGet, path, nil)
	wrong := p.bob.Do(t, http.MethodGet, path+"?since=48h", nil)

	require.Equal(t, http.StatusForbidden, reader.Status, reader.Body)
	require.Contains(t, reader.Body, `"required_scope":"logs"`)
	require.Equal(t, http.StatusForbidden, logsOnly.Status, logsOnly.Body)
	require.Equal(t, http.StatusOK, agent.Status, agent.Body)
	require.Equal(t, http.StatusForbidden, outsider.Status, outsider.Body)
	require.Equal(t, http.StatusBadRequest, wrong.Status)
	require.Contains(t, wrong.Body, `"field":"since"`)
}

func Test_TheDiagnosticsToolsFollowTheTokenScopes(t *testing.T) {
	s, p := setupPlaces(t)

	reader := listTools(t, mcpSession(t, s, issue(t, p.bob, tokenBody("reader", "read")).Token))
	logs := listTools(t, mcpSession(t, s, issue(t, p.bob, tokenBody("logs", "read", "logs")).Token))

	require.Contains(t, reader, "getRuntime")
	require.NotContains(t, reader, "listLogs")
	require.Contains(t, logs, "listLogs")
}

type failingRuntime struct {
	recordingRuntime
	exit string
}

func (r *failingRuntime) Apply(context.Context, agentapi.Workload, io.Writer) error {
	return exec.Command("sh", "-c", "exit "+r.exit).Run()
}

func Test_TheRealAgentSendsTheFailedCandidatesEvidence(t *testing.T) {
	s, p := setupPlaces(t)
	d := deployDev(t, s, p)
	crashed := agentapi.Container{Found: true, Name: "places-api-dev-green", State: "exited", ExitCode: 3, CreatedAt: time.Now()}
	runtime := &failingRuntime{recordingRuntime: recordingRuntime{observed: agent.Observed{Known: true, Container: crashed}}, exit: "10"}
	var view failureView

	require.True(t, realAgent(s, runtime).WorkOnce(context.Background()))
	p.bob.MustJSON(t, http.MethodGet, "/api/v1/deployments/"+d.ID, nil, http.StatusOK, &view)

	require.Equal(t, "crashed", view.Failure.Class)
	require.Equal(t, "places-api-dev-green", view.Failure.Container)
	require.Equal(t, []string{"boot", "crash"}, view.Failure.Logs)
}

func Test_TheRealAgentReportsTheLiveContainer(t *testing.T) {
	s, p := setupPlaces(t)
	deployDev(t, s, p)
	runToSuccess(t, s)
	live := liveContainer()
	a := realAgent(s, &recordingRuntime{observed: agent.Observed{Known: true, Running: true, Container: live}})
	var view runtimeView

	reported := a.StatusOnce(context.Background())
	p.bob.MustJSON(t, http.MethodGet, "/api/v1/runtime/places-api/dev", nil, http.StatusOK, &view)

	require.Equal(t, 1, reported)
	require.Equal(t, "places-api-dev-blue", view.Container)
	require.Equal(t, "running", view.State)
}
