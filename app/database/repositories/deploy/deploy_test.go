package deploy_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/database/repositories/deploy"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	deployment "github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/repositories/mysql"
	"github.com/stretchr/testify/require"
)

var since = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

type latest struct {
	found  bool
	d      deployment.Deployment
	active []deployment.Deployment
}

func (l latest) LatestSucceeded(context.Context, string, string) (deployment.Deployment, bool, error) {
	return l.d, l.found, nil
}

func (l latest) Active(context.Context, string, string) ([]deployment.Deployment, error) {
	return l.active, nil
}

func claimed(at time.Time, status deployment.Status) latest {
	return latest{active: []deployment.Deployment{{Status: status, CreatedAt: at.Add(-time.Minute), StartedAt: &at}}}
}

func startedAt(at time.Time, strategy deployment.Strategy) latest {
	return latest{found: true, d: deployment.Deployment{Strategy: strategy, CreatedAt: at.Add(-time.Minute), StartedAt: &at}}
}

func runsSince(t *testing.T, l latest) bool {
	t.Helper()
	running, err := deploy.NewRunning(l).RunsSince(context.Background(), "svc", "dev", since)
	require.NoError(t, err)
	return running
}

func Test_RunsSince(t *testing.T) {
	var _ deploy.Deployments = mysql.DeploymentRepository{}
	var _ databases.Running = deploy.Running{}
	require.False(t, runsSince(t, latest{}), "none")
	require.False(t, runsSince(t, startedAt(since.Add(-time.Hour), deployment.StrategyRemove)), "removal")
	require.False(t, runsSince(t, startedAt(since.Add(time.Second), deployment.StrategyBlueGreen)), "after")
	require.True(t, runsSince(t, startedAt(since.Add(-time.Hour), deployment.StrategyBlueGreen)), "before")
	require.True(t, runsSince(t, startedAt(since, deployment.StrategyCanary)), "at")
}

func Test_RunsSince_FallsBackToCreatedAt(t *testing.T) {
	never := latest{found: true, d: deployment.Deployment{Strategy: deployment.StrategyBlueGreen, CreatedAt: since.Add(-time.Hour)}}

	require.True(t, runsSince(t, never))
}

func Test_RunsSince_CountsADeploymentClaimedBeforeTheDetach(t *testing.T) {
	require.True(t, runsSince(t, claimed(since.Add(-time.Minute), deployment.StatusInProgress)), "claimed before")
	require.True(t, runsSince(t, claimed(since, deployment.StatusInProgress)), "claimed at")
	require.False(t, runsSince(t, claimed(since.Add(time.Second), deployment.StatusInProgress)), "claimed after")
	require.False(t, runsSince(t, latest{active: []deployment.Deployment{{Status: deployment.StatusPending, CreatedAt: since.Add(-time.Hour)}}}), "pending")
}

func Test_RunsSince_CountsAClaimBehindAnOlderRemoval(t *testing.T) {
	l := claimed(since.Add(-time.Minute), deployment.StatusInProgress)
	l.found, l.d = true, deployment.Deployment{Strategy: deployment.StrategyRemove, CreatedAt: since.Add(-time.Hour)}

	require.True(t, runsSince(t, l))
}
