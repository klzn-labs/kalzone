package deployments_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/deployments"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var apiAndWorker = []deployments.Target{{Service: "api", Environment: "dev"}, {Service: "worker", Environment: "dev"}}

func Test_DeployAll_ResolvesTheReleasePerServiceAndSchedulesEveryTarget(t *testing.T) {
	g := newGated()

	created, err := g.as(authz.Open).DeployAll(ctx, ana, deployments.Batch{Targets: apiAndWorker, Release: "sha:abc1234"})

	require.NoError(t, err)
	require.Equal(t, []string{"r2", "w1"}, []string{created[0].ReleaseID, created[1].ReleaseID})
	require.Equal(t, []string{"s1", "s2"}, []string{created[0].ServiceID, created[1].ServiceID})
	require.Equal(t, entities.StatusPending, created[1].Status)
	require.Equal(t, []string{"deploy.requested", "deploy.requested"}, g.events.actions)
}

func Test_DeployAll_WritesNothingWhenOneTargetFails(t *testing.T) {
	g := newGated()
	g.store.ByID["busy"] = entities.Deployment{ID: "busy", ServiceID: "s2", Environment: "dev", Status: entities.StatusInProgress, CreatedAt: time.Now()}
	targets := append(apiAndWorker, deployments.Target{Service: "api", Environment: "qa"})

	_, err := g.as(authz.Open).DeployAll(ctx, ana, deployments.Batch{Targets: targets, Release: "sha:abc1234"})

	refused := err.(*apperror.AppError)
	require.Equal(t, apperror.CodeConflict, refused.Code)
	require.Equal(t, "targets[1]", refused.Field)
	require.Len(t, refused.Details["targets"], 2)
	require.Len(t, g.store.ByID, 1)
	require.Empty(t, g.events.actions)
}

func Test_DeployAll_RefusesABatchWithATargetThatNeedsApproval(t *testing.T) {
	g := newGated()

	_, err := g.as(authz.NeedsApproval).DeployAll(ctx, ana, deployments.Batch{Targets: apiAndWorker, Release: "sha:abc1234"})

	refused := err.(*apperror.AppError)
	require.Equal(t, apperror.CodeApprovalRequired, refused.Code)
	require.Equal(t, "targets[0]", refused.Field)
	require.Contains(t, refused.Hint, "one at a time")
	require.Empty(t, g.requests.changes)
	require.Empty(t, g.store.ByID)
}

func Test_DeployAll_RefusesAMixedBatchWhenOneTargetNeedsApproval(t *testing.T) {
	g := newGated()
	gates := map[string]authz.Gate{"dev": authz.Open, "prod": authz.NeedsApproval}
	mixed := []deployments.Target{{Service: "api", Environment: "qa"}, {Service: "api", Environment: "dev"}, {Service: "worker", Environment: "prod"}}

	_, err := g.per(gates).DeployAll(ctx, ana, deployments.Batch{Targets: mixed, Release: "sha:abc1234"})

	refused := err.(*apperror.AppError)
	require.Equal(t, apperror.CodeApprovalRequired, refused.Code)
	require.Equal(t, "targets[2]", refused.Field)
	require.Len(t, refused.Details["targets"], 2)
	require.Empty(t, g.requests.changes)
	require.Empty(t, g.store.ByID)
	require.Empty(t, g.events.actions)
}

func Test_DeployAll_ChecksTheTargetList(t *testing.T) {
	g := newGated()
	eleven := make([]deployments.Target, deployments.MaxTargets+1)
	repeated := append(apiAndWorker, apiAndWorker[0])
	incomplete := []deployments.Target{{Service: "api"}}

	_, none := g.as(authz.Open).DeployAll(ctx, ana, deployments.Batch{Release: "r1"})
	_, tooMany := g.as(authz.Open).DeployAll(ctx, ana, deployments.Batch{Targets: eleven, Release: "r1"})
	_, twice := g.as(authz.Open).DeployAll(ctx, ana, deployments.Batch{Targets: repeated, Release: "r1"})
	_, partial := g.as(authz.Open).DeployAll(ctx, ana, deployments.Batch{Targets: incomplete, Release: "r1"})

	require.Equal(t, "targets", none.(*apperror.AppError).Field)
	require.Equal(t, "targets", tooMany.(*apperror.AppError).Field)
	require.Equal(t, "targets[2]", twice.(*apperror.AppError).Field)
	require.Equal(t, "targets[0]", partial.(*apperror.AppError).Field)
}

func Test_PromoteAll_PromotesWhatRunsInTheSourceEnvironment(t *testing.T) {
	g := newGated()
	dev, err := g.as(authz.Open).DeployAll(ctx, ana, deployments.Batch{Targets: apiAndWorker, Release: "sha:abc1234"})
	require.NoError(t, err)
	succeed(g.store, dev[0])
	succeed(g.store, dev[1])
	prod := []deployments.Target{{Service: "api", Environment: "prod"}, {Service: "worker", Environment: "prod"}}

	promoted, err := g.as(authz.Bypassed).PromoteAll(ctx, ana, prod, "dev")

	require.NoError(t, err)
	require.Equal(t, []string{"r2", "w1"}, []string{promoted[0].ReleaseID, promoted[1].ReleaseID})
	require.Equal(t, entities.TriggerPromote, promoted[0].Trigger)
	require.Equal(t, 2, promoted[1].ConfigRevision)
	require.Equal(t, []string{"deploy.requested", "token.bypass_used", "deploy.requested", "token.bypass_used"}, g.events.actions[2:])
}
