package deployments_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/app/deploy/usecases/deployments"
	"github.com/adrianozp/platform/app/deploy/usecases/fakestore"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/events"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()
var ana = actor.Actor{UserID: "ana", Login: "ana"}

type releases map[string]entities.ReleaseRef

func (r releases) FindRelease(_ context.Context, id string) (entities.ReleaseRef, error) {
	rel, ok := r[id]
	if !ok {
		return entities.ReleaseRef{}, apperror.NotFound("release not found")
	}
	return rel, nil
}

type configs map[string]int

func (c configs) CurrentNumber(_ context.Context, _, env string) (int, error) { return c[env], nil }

type environments struct{}

func (environments) EnvironmentExists(_ context.Context, slug string) error {
	if slug != "dev" && slug != "prod" {
		return apperror.NotFound("environment not found")
	}
	return nil
}

type policy struct{ denyEnv string }

func (p policy) Can(_ context.Context, _ actor.Actor, action authz.Action, r authz.Resource) error {
	if action != authz.Deploy || r.Environment == p.denyEnv {
		return apperror.Forbidden("no")
	}
	return nil
}

type published struct{ actions []string }

func (p *published) Publish(_ context.Context, e events.Event) {
	p.actions = append(p.actions, e.Action)
}

var rels = releases{
	"r1":    {ID: "r1", ServiceID: "s1", Number: 1, Built: true, Reference: "img@sha256:1"},
	"r2":    {ID: "r2", ServiceID: "s1", Number: 2, Built: true, Reference: "img@sha256:2"},
	"wip":   {ID: "wip", ServiceID: "s1", Number: 3, Built: false},
	"other": {ID: "other", ServiceID: "s9", Number: 1, Built: true},
}

func setup(denyEnv string) (*fakestore.Store, deployments.DeploymentsUsecase) {
	store := fakestore.New()
	usecase := deployments.NewDeploymentsUsecase(store, fakestore.Services{}, rels, configs{"dev": 4, "prod": 2}, environments{}, policy{denyEnv}, &published{})
	return store, usecase
}

func succeed(store *fakestore.Store, d entities.Deployment) {
	now := time.Now()
	d.Status, d.FinishedAt = entities.StatusSucceeded, &now
	store.ByID[d.ID] = d
}

func Test_Deploy_UsesTheCurrentConfigRevision(t *testing.T) {
	_, usecase := setup("")

	d, err := usecase.Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "dev", ReleaseID: "r1"})

	require.NoError(t, err)
	require.Equal(t, entities.StatusPending, d.Status)
	require.Equal(t, 4, d.ConfigRevision)
	require.Equal(t, "img@sha256:1", d.ImageRef)
	require.Equal(t, entities.StrategyBlueGreen, d.Strategy)
}

func Test_Deploy_Refusals(t *testing.T) {
	_, usecase := setup("prod")
	cases := map[string]struct {
		req  deployments.Request
		want error
	}{
		"unbuilt":       {deployments.Request{Service: "api", Environment: "dev", ReleaseID: "wip"}, apperror.Invalid("")},
		"other service": {deployments.Request{Service: "api", Environment: "dev", ReleaseID: "other"}, apperror.Invalid("")},
		"strategy":      {deployments.Request{Service: "api", Environment: "dev", ReleaseID: "r1", Strategy: "yolo"}, apperror.Invalid("")},
		"no env":        {deployments.Request{Service: "api", Environment: "qa", ReleaseID: "r1"}, apperror.NotFound("")},
		"forbidden":     {deployments.Request{Service: "api", Environment: "prod", ReleaseID: "r1"}, apperror.Forbidden("")},
	}
	for name, c := range cases {
		_, err := usecase.Deploy(ctx, ana, c.req)
		require.ErrorIs(t, err, c.want, name)
	}
}

func Test_Deploy_SupersedesPendingButNotRunning(t *testing.T) {
	store, usecase := setup("")
	first, _ := usecase.Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "dev", ReleaseID: "r1"})

	second, err := usecase.Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "dev", ReleaseID: "r2"})
	require.NoError(t, err)
	running := store.ByID[second.ID]
	running.Status = entities.StatusInProgress
	store.ByID[second.ID] = running
	_, blocked := usecase.Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "dev", ReleaseID: "r1"})

	require.Equal(t, entities.StatusCancelled, store.ByID[first.ID].Status)
	require.ErrorIs(t, blocked, apperror.Conflict(""))
}

func Test_Promote_TakesTheSourceReleaseAndTheTargetConfig(t *testing.T) {
	store, usecase := setup("")
	dev, _ := usecase.Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "dev", ReleaseID: "r2"})
	succeed(store, dev)

	prod, err := usecase.Promote(ctx, ana, "api", "dev", "prod")
	_, nothing := usecase.Promote(ctx, ana, "api", "prod", "dev")

	require.NoError(t, err)
	require.Equal(t, "r2", prod.ReleaseID)
	require.Equal(t, 2, prod.ConfigRevision)
	require.Equal(t, entities.TriggerPromote, prod.Trigger)
	require.ErrorIs(t, nothing, apperror.Invalid(""))
}

func Test_Rollback_ReusesTheOldConfigRevision(t *testing.T) {
	store, usecase := setup("")
	old, _ := usecase.Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "dev", ReleaseID: "r1"})
	old.ConfigRevision = 1
	succeed(store, old)
	newer, _ := usecase.Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "dev", ReleaseID: "r2"})
	succeed(store, newer)

	back, err := usecase.Rollback(ctx, ana, old.ID)

	require.NoError(t, err)
	require.Equal(t, "r1", back.ReleaseID)
	require.Equal(t, 1, back.ConfigRevision)
	require.Equal(t, entities.TriggerRollback, back.Trigger)
}

func Test_ApplyConfig_RedeploysTheLiveReleaseWithNewConfig(t *testing.T) {
	store, usecase := setup("")
	live, _ := usecase.Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "dev", ReleaseID: "r2"})
	live.ConfigRevision = 3
	succeed(store, live)

	applied, err := usecase.ApplyConfig(ctx, ana, "api", "dev")
	require.NoError(t, err)
	succeed(store, applied)
	_, again := usecase.ApplyConfig(ctx, ana, "api", "dev")

	require.Equal(t, "r2", applied.ReleaseID)
	require.Equal(t, 4, applied.ConfigRevision)
	require.ErrorIs(t, again, apperror.Invalid(""))
}
