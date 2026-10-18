package agent_test

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/agent"
	"github.com/klzn-labs/kalzone/pkg/agentapi"
	"github.com/stretchr/testify/require"
)

func exitError(t *testing.T, code string) error {
	t.Helper()
	err := exec.Command("sh", "-c", "exit "+code).Run()
	require.Error(t, err)
	return err
}

func failingRun(t *testing.T, r runtime) *agentapi.Failure {
	t.Helper()
	p := &plane{work: []agentapi.Workload{{DeploymentID: "d1", Name: "api-dev", ImageRef: "img@1"}}, finished: map[string]string{}, failures: map[string]*agentapi.Failure{}}
	newAgent(p, r).WorkOnce(ctx)
	return p.failures["d1"]
}

func Test_AFailedDeploySendsTheNewestCandidatesEvidence(t *testing.T) {
	now := time.Now()
	r := runtime{
		failWith: exitError(t, "10"),
		containers: map[string]agentapi.Container{
			"api-dev-blue":  {Found: true, Name: "api-dev-blue", State: "running", Health: "healthy", CreatedAt: now.Add(-time.Hour)},
			"api-dev-green": {Found: true, Name: "api-dev-green", State: "exited", Health: "unhealthy", HealthLog: []string{"404"}, CreatedAt: now},
		},
		logs: []string{"listening"},
	}

	failure := failingRun(t, r)

	require.Equal(t, "unhealthy", failure.Class)
	require.Equal(t, "api-dev-green", failure.Container)
	require.Equal(t, []string{"404"}, failure.HealthLog)
	require.Equal(t, []string{"listening"}, failure.Logs)
}

func Test_AFailedDeployWithoutAContainerFailedToStart(t *testing.T) {
	failure := failingRun(t, runtime{failWith: exitError(t, "10")})

	require.Equal(t, "start_failed", failure.Class)
	require.Empty(t, failure.Container)
	require.Empty(t, failure.Logs)
}

type classifyCase struct {
	name string
	err  error
	ctx  error
	c    agentapi.Container
	want string
}

func classifyCases(t *testing.T) []classifyCase {
	crashed := agentapi.Container{Found: true, State: "exited", ExitCode: 2}
	return []classifyCase{
		{"timeout", exitError(t, "10"), context.DeadlineExceeded, crashed, "timeout"},
		{"oom", exitError(t, "10"), nil, agentapi.Container{Found: true, OOMKilled: true}, "oom_killed"},
		{"unhealthy", exitError(t, "10"), nil, agentapi.Container{Found: true, State: "running", Health: "unhealthy"}, "unhealthy"},
		{"crashed", exitError(t, "10"), nil, crashed, "crashed"},
		{"start", exitError(t, "10"), nil, agentapi.Container{Found: true, State: "running"}, "start_failed"},
		{"smoke", exitError(t, "11"), nil, agentapi.Container{}, "smoke_failed"},
		{"ready", exitError(t, "14"), nil, agentapi.Container{Found: true, State: "running"}, "unhealthy"},
		{"gate", exitError(t, "12"), nil, agentapi.Container{}, "gate_failed"},
		{"other", exitError(t, "13"), nil, agentapi.Container{}, "platform_error"},
		{"plain", errors.New("deployctl missing"), nil, agentapi.Container{}, "platform_error"},
	}
}

func Test_Classify_PicksTheFirstMatchingClass(t *testing.T) {
	for _, c := range classifyCases(t) {
		require.Equal(t, c.want, agent.Classify(c.err, c.ctx, c.c), c.name)
	}
}

func Test_StatusOnce_ReportsEveryDesiredWorkloadInOneBatch(t *testing.T) {
	p := &plane{desired: []agentapi.Desired{
		{Name: "a-dev", Service: "a", Environment: "dev"},
		{Name: "b-dev", Service: "b", Environment: "dev"},
	}}
	live := agentapi.Container{Found: true, Name: "a-dev-blue", State: "running"}
	r := runtime{observed: map[string]agent.Observed{"a-dev": {Known: true, Running: true, Container: live}}}

	reported := newAgent(p, r).StatusOnce(ctx)

	require.Equal(t, 2, reported)
	require.Len(t, p.reports, 1)
	require.Equal(t, agentapi.Snapshot{Service: "a", Environment: "dev", Container: live}, p.reports[0][0])
	require.False(t, p.reports[0][1].Container.Found)
}
