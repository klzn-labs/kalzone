package agent_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/adrianozp/platform/internal/agent"
	"github.com/adrianozp/platform/pkg/agentapi"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type plane struct {
	work     []agentapi.Workload
	lines    []string
	finished map[string]string
	desired  []agentapi.Desired
	drifts   []agentapi.Drift
	failures map[string]*agentapi.Failure
	reports  [][]agentapi.Snapshot
}

func (p *plane) Claim(context.Context) (agentapi.Workload, bool, error) {
	if len(p.work) == 0 {
		return agentapi.Workload{}, false, nil
	}
	w := p.work[0]
	p.work = p.work[1:]
	return w, true, nil
}

func (p *plane) Events(_ context.Context, _ string, lines []string) error {
	p.lines = append(p.lines, lines...)
	return nil
}

func (p *plane) Finish(_ context.Context, id string, ok bool, message string, failure *agentapi.Failure) error {
	p.finished[id] = fmt.Sprintf("%v %s", ok, message)
	if p.failures != nil {
		p.failures[id] = failure
	}
	return nil
}

func (p *plane) Status(_ context.Context, snapshots []agentapi.Snapshot) error {
	p.reports = append(p.reports, snapshots)
	return nil
}

func (p *plane) Desired(context.Context) ([]agentapi.Desired, error) { return p.desired, nil }

func (p *plane) Drift(_ context.Context, d agentapi.Drift) error {
	p.drifts = append(p.drifts, d)
	return nil
}

type runtime struct {
	fail       bool
	failWith   error
	observed   map[string]agent.Observed
	containers map[string]agentapi.Container
	logs       []string
}

func (r runtime) Apply(_ context.Context, w agentapi.Workload, out io.Writer) error {
	fmt.Fprintf(out, "pulling %s\nswitching traffic\nhalf a line", w.ImageRef)
	if r.failWith != nil {
		return r.failWith
	}
	if r.fail {
		return errors.New("rollback: 5xx above threshold")
	}
	return nil
}

func (r runtime) Inspect(_ context.Context, container string) (agentapi.Container, error) {
	return r.containers[container], nil
}

func (r runtime) Logs(context.Context, string, int) ([]string, error) {
	return r.logs, nil
}

func (r runtime) Observe(_ context.Context, name string) (agent.Observed, error) {
	return r.observed[name], nil
}

func newAgent(p *plane, r runtime) agent.Agent {
	return agent.New(agent.Config{}, p, r)
}

func Test_WorkOnce_RunsAndReportsSuccess(t *testing.T) {
	p := &plane{work: []agentapi.Workload{{DeploymentID: "d1", Name: "api-dev", ImageRef: "img@1"}}, finished: map[string]string{}}

	worked := newAgent(p, runtime{}).WorkOnce(ctx)
	idle := newAgent(p, runtime{}).WorkOnce(ctx)

	require.True(t, worked)
	require.False(t, idle)
	require.Equal(t, []string{"pulling img@1", "switching traffic", "half a line"}, p.lines)
	require.Equal(t, "true live", p.finished["d1"])
}

func Test_WorkOnce_ReportsFailureWithTheLastLines(t *testing.T) {
	p := &plane{work: []agentapi.Workload{{DeploymentID: "d1", Name: "api-dev", ImageRef: "img@1"}}, finished: map[string]string{}}

	newAgent(p, runtime{fail: true}).WorkOnce(ctx)

	require.Contains(t, p.finished["d1"], "false rollback: 5xx above threshold")
	require.Contains(t, p.finished["d1"], "switching traffic")
}

func Test_ReconcileOnce_ReportsOnlyDifferences(t *testing.T) {
	p := &plane{desired: []agentapi.Desired{
		{Name: "a-dev", Service: "a", Environment: "dev", ImageRef: "img@1"},
		{Name: "b-dev", Service: "b", Environment: "dev", ImageRef: "img@2"},
		{Name: "c-dev", Service: "c", Environment: "dev", ImageRef: "img@3"},
		{Name: "d-dev", Service: "d", Environment: "dev", ImageRef: "img@4"},
	}}
	r := runtime{observed: map[string]agent.Observed{
		"a-dev": {Known: true, Running: true, Image: "img@1"},
		"b-dev": {Known: true, Running: false, Image: "img@2"},
		"c-dev": {Known: true, Running: true, Image: "img@old"},
	}}

	reported := newAgent(p, r).ReconcileOnce(ctx)

	require.Equal(t, 3, reported)
	require.Equal(t, "b-dev is not running", p.drifts[0].Observed)
	require.Equal(t, "c-dev runs img@old instead of img@3", p.drifts[1].Observed)
	require.Equal(t, "d-dev has never been deployed on this host", p.drifts[2].Observed)
}
