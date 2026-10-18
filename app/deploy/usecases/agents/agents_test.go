package agents_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/agents"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/fakestore"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type resolver struct{ fail bool }

func (r resolver) ResolveConfig(_ context.Context, _, _ string, number int) (entities.ResolvedConfig, error) {
	if r.fail {
		return entities.ResolvedConfig{}, apperror.Invalid("secret could not be opened")
	}
	return entities.ResolvedConfig{Number: number, Secrets: map[string]string{"TOKEN": "s3cr3t"}}, nil
}

type queueAccess struct{}

func (queueAccess) QueueAccess(_ context.Context, serviceID, environment string) (map[string]string, map[string]string, error) {
	return map[string]string{"PLATFORM_QUEUE_URL": "http://queued:8080"},
		map[string]string{"PLATFORM_QUEUE_TOKEN": "token-" + serviceID + "-" + environment}, nil
}

type published struct{ actions []string }

func (p *published) Publish(_ context.Context, e events.Event) {
	p.actions = append(p.actions, e.Action)
}

func setup(fail bool) (*fakestore.Store, *published, agents.AgentsUsecase) {
	store, p := fakestore.New(), &published{}
	cfg := config.Config{Deploy: config.Deploy{Timeout: 30 * time.Minute}}
	return store, p, agents.NewAgentsUsecase(cfg, store, fakestore.Services{}, resolver{fail}, queueAccess{}, p)
}

func pending(store *fakestore.Store, id, env string, status entities.Status) entities.Deployment {
	d, _ := store.Create(ctx, entities.Deployment{
		ID: id, ServiceID: "s1", Environment: env, ReleaseID: "r1", ReleaseNumber: 7, ImageRef: "img@sha256:1",
		ConfigRevision: 3, Strategy: entities.StrategyBlueGreen, Status: status, CreatedAt: time.Now(),
	})
	return d
}

func Test_Claim_HandsOutTheOldestPendingOnce(t *testing.T) {
	store, _, usecase := setup(false)
	pending(store, "d1", "dev", entities.StatusPending)

	w, claimed, err := usecase.Claim(ctx, "vps-1")
	_, again, _ := usecase.Claim(ctx, "vps-2")

	require.NoError(t, err)
	require.True(t, claimed)
	require.False(t, again)
	require.Equal(t, "api-dev", w.Name)
	require.Equal(t, "r7.c3", w.Version)
	require.Equal(t, "s3cr3t", w.Config.Secrets["TOKEN"])
	require.Equal(t, entities.StatusInProgress, store.ByID["d1"].Status)
	require.Equal(t, "vps-1", store.ByID["d1"].Agent)
}

func Test_Claim_AddsQueueAccessToTheWorkload(t *testing.T) {
	store, _, usecase := setup(false)
	pending(store, "d1", "dev", entities.StatusPending)

	w, _, err := usecase.Claim(ctx, "vps-1")

	require.NoError(t, err)
	require.Equal(t, "http://queued:8080", w.Config.Env["PLATFORM_QUEUE_URL"])
	require.Equal(t, "token-s1-dev", w.Config.Secrets["PLATFORM_QUEUE_TOKEN"])
	require.Equal(t, "s3cr3t", w.Config.Secrets["TOKEN"])
}

func Test_Claim_FailsAWorkloadThatCannotBePrepared(t *testing.T) {
	store, _, usecase := setup(true)
	pending(store, "d1", "dev", entities.StatusPending)

	_, claimed, err := usecase.Claim(ctx, "vps-1")

	require.NoError(t, err)
	require.False(t, claimed)
	require.Equal(t, entities.StatusFailed, store.ByID["d1"].Status)
	require.Contains(t, store.ByID["d1"].Message, "could not prepare")
}

func Test_ReportAndFinish_OnlyByTheClaimingAgent(t *testing.T) {
	store, p, usecase := setup(false)
	pending(store, "d1", "dev", entities.StatusPending)
	_, _, _ = usecase.Claim(ctx, "vps-1")

	stranger := usecase.Report(ctx, "vps-2", "d1", []string{"x"})
	require.NoError(t, usecase.Report(ctx, "vps-1", "d1", []string{"pulling\n", "switching traffic"}))
	done, err := usecase.Finish(ctx, "vps-1", "d1", true, "live", nil)
	_, twice := usecase.Finish(ctx, "vps-1", "d1", true, "live", nil)

	require.ErrorIs(t, stranger, apperror.Forbidden(""))
	require.NoError(t, err)
	require.Equal(t, entities.StatusSucceeded, done.Status)
	require.Equal(t, []string{"pulling", "switching traffic"}, store.Lines["d1"])
	require.ErrorIs(t, twice, apperror.Conflict(""))
	require.Equal(t, []string{"deploy.succeeded"}, p.actions)
}

func Test_Desired_IsTheLatestSuccessPerEnvironment(t *testing.T) {
	store, _, usecase := setup(false)
	pending(store, "d1", "dev", entities.StatusSucceeded)
	pending(store, "d2", "dev", entities.StatusSucceeded)
	pending(store, "p1", "prod", entities.StatusFailed)

	desired, err := usecase.Desired(ctx)

	require.NoError(t, err)
	require.Len(t, desired, 1)
	require.Equal(t, entities.Desired{Name: "api-dev", Service: "api", Environment: "dev", ImageRef: "img@sha256:1", DeploymentID: "d2"}, desired[0])
}

func Test_Drift_CreatesOneReconcileDeployment(t *testing.T) {
	store, p, usecase := setup(false)
	pending(store, "d1", "dev", entities.StatusSucceeded)

	d, created, err := usecase.Drift(ctx, "vps-1", entities.Drift{Service: "api", Environment: "dev", Observed: "container missing"})
	_, again, _ := usecase.Drift(ctx, "vps-1", entities.Drift{Service: "api", Environment: "dev", Observed: "container missing"})
	_, nothingLive, _ := usecase.Drift(ctx, "vps-1", entities.Drift{Service: "api", Environment: "prod"})

	require.NoError(t, err)
	require.True(t, created)
	require.False(t, again)
	require.False(t, nothingLive)
	require.Equal(t, entities.TriggerReconcile, d.Trigger)
	require.Equal(t, 3, d.ConfigRevision)
	require.Equal(t, []string{"deploy.drift"}, p.actions)
}

func Test_Sweep_FailsDeploymentsWhoseAgentWentQuiet(t *testing.T) {
	store, _, usecase := setup(false)
	pending(store, "d1", "dev", entities.StatusPending)
	clock.WithTime(time.Now().Add(-time.Hour))
	_, _, _ = usecase.Claim(ctx, "vps-1")
	clock.WithTime(time.Now())

	swept, err := usecase.Sweep(ctx)

	require.NoError(t, err)
	require.Equal(t, 1, swept)
	require.Equal(t, "the agent stopped reporting", store.ByID["d1"].Message)
}

func removal(store *fakestore.Store, id, env string, status entities.Status) {
	d := pending(store, id, env, status)
	d.Strategy, d.Trigger, d.ImageRef = entities.StrategyRemove, entities.TriggerDecommission, ""
	store.ByID[id] = d
}

func Test_Claim_HandsOutARemovalWithoutConfigOrQueueAccess(t *testing.T) {
	store, _, usecase := setup(true)
	removal(store, "rm1", "dev", entities.StatusPending)

	w, claimed, err := usecase.Claim(ctx, "vps-1")

	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, "api-dev", w.Name)
	require.Equal(t, entities.StrategyRemove, w.Deployment.Strategy)
	require.Empty(t, w.Config.Secrets)
	require.Empty(t, w.Config.Env)
}

func Test_Desired_DropsAWorkloadOnceItsRemovalSucceeded(t *testing.T) {
	store, _, usecase := setup(false)
	pending(store, "d1", "dev", entities.StatusSucceeded)
	removal(store, "rm1", "dev", entities.StatusSucceeded)
	pending(store, "p1", "prod", entities.StatusSucceeded)

	desired, err := usecase.Desired(ctx)

	require.NoError(t, err)
	require.Len(t, desired, 1)
	require.Equal(t, "prod", desired[0].Environment)
}
