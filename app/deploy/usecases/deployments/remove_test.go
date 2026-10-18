package deployments_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/deployments"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/fakestore"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/stretchr/testify/require"
)

func retiredLive(store *fakestore.Store, id, env string, config int) entities.Deployment {
	d, _ := store.Create(ctx, entities.Deployment{
		ID: id, ServiceID: fakestore.Retired.ID, Environment: env, ReleaseID: "r9", ReleaseNumber: 9, ImageRef: "img@sha256:9",
		ConfigRevision: config, Strategy: entities.StrategyBlueGreen, Trigger: entities.TriggerManual, Status: entities.StatusPending,
		CreatedAt: time.Now(),
	})
	succeed(store, d)
	return store.ByID[id]
}

func Test_Remove_SchedulesATeardownOfTheLiveWorkload(t *testing.T) {
	g := newGated()
	retiredLive(g.store, "d1", "dev", 7)

	d, err := g.as(authz.Open).Remove(ctx, ana, "retired", "dev")

	require.NoError(t, err)
	require.Equal(t, entities.StrategyRemove, d.Strategy)
	require.Equal(t, entities.TriggerDecommission, d.Trigger)
	require.Equal(t, entities.StatusPending, d.Status)
	require.Equal(t, 7, d.ConfigRevision)
	require.Empty(t, d.ImageRef)
}

func Test_Remove_OnAGatedEnvironmentBecomesAChangeRequest(t *testing.T) {
	g := newGated()
	retiredLive(g.store, "p1", "prod", 2)

	_, err := g.as(authz.NeedsApproval).Remove(ctx, ana, "retired", "prod")

	var pending *approval.Pending
	require.ErrorAs(t, err, &pending)
	require.Equal(t, approval.KindRemove, g.requests.changes[0].Kind)
	require.Equal(t, "remove retired from prod", g.requests.changes[0].Summary)
}

func Test_AnApprovedRemovalIsScheduled(t *testing.T) {
	g := newGated()
	retiredLive(g.store, "p1", "prod", 2)
	_, _ = g.as(authz.NeedsApproval).Remove(ctx, ana, "retired", "prod")

	d, err := g.as(authz.Approved).ApplyApproved(ctx, approvedAna, g.proposal(t))

	require.NoError(t, err)
	require.Equal(t, entities.StrategyRemove, d.Strategy)
}

func Test_ADecommissioningServiceRefusesEveryOtherChange(t *testing.T) {
	g := newGated()
	live := retiredLive(g.store, "d1", "dev", 7)
	usecase := g.as(authz.Open)

	_, deploy := usecase.Deploy(ctx, ana, deployments.Request{Service: "retired", Environment: "dev", ReleaseID: "r1"})
	_, apply := usecase.ApplyConfig(ctx, ana, "retired", "dev")
	_, rollback := usecase.Rollback(ctx, ana, live.ID)
	_, auto := usecase.AutoDeploy(ctx, fakestore.Retired.ID, "r1")

	for _, err := range []error{deploy, apply, rollback, auto} {
		require.ErrorIs(t, err, apperror.Conflict(""))
	}
}

func Test_Teardown_ReportsWhichEnvironmentsAreRemoved(t *testing.T) {
	g := newGated()
	retiredLive(g.store, "d1", "dev", 7)
	retiredLive(g.store, "p1", "prod", 2)
	removal, _ := g.as(authz.Open).Remove(ctx, ana, "retired", "dev")

	before, _ := g.as(authz.Open).Teardown(ctx, fakestore.Retired.ID)
	removing, _ := g.as(authz.Open).Removing(ctx, fakestore.Retired.ID, "dev")
	succeed(g.store, removal)
	after, err := g.as(authz.Open).Teardown(ctx, fakestore.Retired.ID)

	require.NoError(t, err)
	require.Equal(t, map[string]bool{"dev": false, "prod": false}, before)
	require.True(t, removing)
	require.Equal(t, map[string]bool{"dev": true, "prod": false}, after)
}

func pendingRemoval(store *fakestore.Store, id, serviceID, env string) entities.Deployment {
	d, _ := store.Create(ctx, entities.Deployment{
		ID: id, ServiceID: serviceID, Environment: env, Strategy: entities.StrategyRemove,
		Trigger: entities.TriggerDecommission, Status: entities.StatusPending, CreatedAt: time.Now(),
	})
	return d
}

func Test_RollingBackToARemovalIsRefused(t *testing.T) {
	g := newGated()
	g.live(t, "dev", "r1")
	removal := pendingRemoval(g.store, "rm1", fakestore.API.ID, "dev")
	succeed(g.store, removal)

	_, err := g.as(authz.Open).Rollback(ctx, ana, removal.ID)

	require.ErrorIs(t, err, apperror.Invalid(""))
}

func Test_Freeze_CancelsPendingChangesButKeepsRemovals(t *testing.T) {
	g := newGated()
	g.live(t, "dev", "r1")
	pendingDeploy, _ := g.as(authz.Open).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "dev", ReleaseID: "r2"})
	removal := pendingRemoval(g.store, "rm1", fakestore.API.ID, "dev")

	require.NoError(t, g.as(authz.Open).Freeze(ctx, fakestore.API.ID))

	require.Equal(t, entities.StatusCancelled, g.store.ByID[pendingDeploy.ID].Status)
	require.Equal(t, entities.StatusPending, g.store.ByID[removal.ID].Status)
}

func Test_Freeze_RefusesWhileADeploymentRuns(t *testing.T) {
	g := newGated()
	running, _ := g.as(authz.Open).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "dev", ReleaseID: "r1"})
	running.Status = entities.StatusInProgress
	g.store.ByID[running.ID] = running

	err := g.as(authz.Open).Freeze(ctx, fakestore.API.ID)

	require.ErrorIs(t, err, apperror.Conflict(""))
}

func Test_ForgetRuntimeDropsTheServicesRows(t *testing.T) {
	g := newGated()
	require.NoError(t, g.store.SaveRuntime(ctx, []entities.RuntimeStatus{{ServiceID: "s3", Environment: "dev"}}))

	require.NoError(t, g.as(authz.Open).ForgetRuntime(ctx, "s3"))

	require.Empty(t, g.store.Runtimes)
}
