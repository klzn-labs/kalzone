package tests

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/klzn-labs/kalzone/internal/agent"
	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/klzn-labs/kalzone/pkg/agentapi"
	"github.com/stretchr/testify/require"
)

type recordingRuntime struct {
	applied  []agentapi.Workload
	observed agent.Observed
}

func (r *recordingRuntime) Apply(_ context.Context, w agentapi.Workload, out io.Writer) error {
	r.applied = append(r.applied, w)
	fmt.Fprintln(out, "deployctl: "+w.Name+" is live")
	return nil
}

func (r *recordingRuntime) Observe(context.Context, string) (agent.Observed, error) {
	return r.observed, nil
}

func (r *recordingRuntime) Inspect(_ context.Context, container string) (agentapi.Container, error) {
	if container == r.observed.Container.Name {
		return r.observed.Container, nil
	}
	return agentapi.Container{}, nil
}

func (r *recordingRuntime) Platform(context.Context, string) (agentapi.PlatformReport, bool, error) {
	return agentapi.PlatformReport{}, false, nil
}

func (r *recordingRuntime) Logs(context.Context, string, int) ([]string, error) {
	return []string{"boot", "crash"}, nil
}

func realAgent(s *integration.Suite, runtime agent.Runtime) agent.Agent {
	cfg := agent.Config{PlatformURL: s.Server.URL, Token: integration.AgentToken, Name: "vps-1", Version: "test"}
	return agent.New(cfg, agent.NewClient(cfg), runtime)
}

func Test_TheAgentDeploysAndThenHealsDrift(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "dev", "release": releaseID}, http.StatusAccepted, nil)
	runtime := &recordingRuntime{}
	a := realAgent(s, runtime)

	require.True(t, a.WorkOnce(context.Background()))
	require.Equal(t, 1, a.ReconcileOnce(context.Background()))
	require.True(t, a.WorkOnce(context.Background()))

	require.Len(t, runtime.applied, 2)
	var history struct {
		Deployments []deploymentView `json:"items"`
	}
	p.bob.MustJSON(t, http.MethodGet, "/api/v1/deployments?service=places-api&env=dev", nil, http.StatusOK, &history)
	require.Equal(t, "reconcile", history.Deployments[0].Trigger)
	require.Equal(t, "succeeded", history.Deployments[0].Status)
	require.Equal(t, "succeeded", history.Deployments[1].Status)
}
