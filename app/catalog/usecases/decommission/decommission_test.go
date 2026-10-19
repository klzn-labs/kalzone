package decommission_test

import (
	"context"
	"errors"
	"testing"

	"github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/decommission"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/stretchr/testify/require"
)

var (
	ctx = context.Background()
	mia = actor.Actor{UserID: "u-mia", Login: "mia"}
)

type world struct {
	service   entities.Service
	removed   map[string]bool
	removing  map[string]bool
	gated     map[string]bool
	freezeErr error
	dashErr   error
	calls     []string
}

func (w *world) record(call string) { w.calls = append(w.calls, call) }

func (w *world) MarkDecommissioning(_ context.Context, _ actor.Actor, slug string) (entities.Service, error) {
	if slug != w.service.Slug {
		return entities.Service{}, apperror.NotFound("service not found")
	}
	w.service.Status = entities.StatusDecommissioning
	w.record("mark decommissioning")
	return w.service, nil
}

func (w *world) MarkDeleted(_ context.Context, a actor.Actor, _ string) error {
	w.record("mark deleted by " + a.Login)
	return nil
}

func (w *world) GetByID(context.Context, string) (entities.Service, error) { return w.service, nil }

func (w *world) Freeze(context.Context, string) error {
	w.record("freeze")
	return w.freezeErr
}

func (w *world) Teardown(context.Context, string) (map[string]bool, error) {
	copied := map[string]bool{}
	for env, done := range w.removed {
		copied[env] = done
	}
	return copied, nil
}

func (w *world) Removing(_ context.Context, _, env string) (bool, error) { return w.removing[env], nil }

func (w *world) Remove(_ context.Context, _ actor.Actor, _, env string) (entities.TeardownStep, error) {
	w.record("remove " + env)
	if w.gated[env] {
		return entities.TeardownStep{Environment: env, Status: entities.TeardownAwaitingApproval, RequestID: "c1"}, nil
	}
	return entities.TeardownStep{Environment: env, Status: entities.TeardownRemoving, DeploymentID: "rm-" + env}, nil
}

func (w *world) ForgetRuntime(context.Context, string) error {
	w.record("forget runtime")
	return nil
}

func (w *world) FailUnfinished(context.Context, actor.Actor, string) error {
	w.record("fail builds")
	return nil
}

func (w *world) CancelForService(context.Context, actor.Actor, string) error {
	w.record("cancel requests")
	return nil
}

func (w *world) Detach(context.Context, actor.Actor, string) error {
	w.record("detach queues")
	return nil
}

func (w *world) DetachService(context.Context, actor.Actor, string) error {
	w.record("detach databases")
	return nil
}

func (w *world) Delete(context.Context, string) error {
	w.record("delete dashboard")
	return w.dashErr
}

func newWorld(removed map[string]bool) *world {
	return &world{
		service: entities.Service{ID: "s1", Slug: "api", Status: entities.StatusActive},
		removed: removed, removing: map[string]bool{}, gated: map[string]bool{},
	}
}

func usecase(w *world) decommission.DecommissionUsecase {
	return decommission.NewDecommissionUsecase(w, w, w, w, w, w, w)
}

var finishCalls = []string{"detach queues", "detach databases", "forget runtime", "delete dashboard", "mark deleted by decommission"}

func Test_Start_DeletesAServiceThatNeverRanAtOnce(t *testing.T) {
	w := newWorld(map[string]bool{})

	result, err := usecase(w).Start(ctx, mia, "api")

	require.NoError(t, err)
	require.True(t, result.Deleted)
	require.Empty(t, result.Steps)
	want := append([]string{"mark decommissioning", "freeze", "cancel requests", "fail builds"}, finishCalls...)
	require.Equal(t, want, w.calls)
}

func Test_Start_TearsDownEveryEnvironmentAndWaitsForApprovalOnProd(t *testing.T) {
	w := newWorld(map[string]bool{"dev": false, "prod": false})
	w.gated["prod"] = true

	result, err := usecase(w).Start(ctx, mia, "api")

	require.NoError(t, err)
	require.False(t, result.Deleted)
	require.Equal(t, []entities.TeardownStep{
		{Environment: "dev", Status: entities.TeardownRemoving, DeploymentID: "rm-dev"},
		{Environment: "prod", Status: entities.TeardownAwaitingApproval, RequestID: "c1"},
	}, result.Steps)
	require.NotContains(t, w.calls, "detach queues")
}

func Test_Start_AgainResumesWithoutDuplicatingRemovals(t *testing.T) {
	w := newWorld(map[string]bool{"dev": true, "prod": false})
	w.removing["prod"] = true

	result, err := usecase(w).Start(ctx, mia, "api")

	require.NoError(t, err)
	require.Equal(t, []entities.TeardownStep{
		{Environment: "dev", Status: entities.TeardownRemoved},
		{Environment: "prod", Status: entities.TeardownRemoving},
	}, result.Steps)
	require.NotContains(t, w.calls, "remove prod")
	require.NotContains(t, w.calls, "remove dev")
}

func Test_Start_StopsBeforeCancellingWhenADeploymentRuns(t *testing.T) {
	w := newWorld(map[string]bool{"dev": false})
	w.freezeErr = apperror.Conflict("a deployment is already running here")

	_, err := usecase(w).Start(ctx, mia, "api")

	require.ErrorIs(t, err, apperror.Conflict(""))
	require.Equal(t, []string{"mark decommissioning", "freeze"}, w.calls)
}

func Test_Finish_WaitsForEveryEnvironment(t *testing.T) {
	w := newWorld(map[string]bool{"dev": true, "prod": false})
	w.service.Status = entities.StatusDecommissioning

	deleted, err := usecase(w).Finish(ctx, "s1")

	require.NoError(t, err)
	require.False(t, deleted)
	require.Empty(t, w.calls)
}

func Test_Finish_LeavesAnActiveServiceAlone(t *testing.T) {
	w := newWorld(map[string]bool{"dev": true})

	deleted, err := usecase(w).Finish(ctx, "s1")

	require.NoError(t, err)
	require.False(t, deleted)
	require.Empty(t, w.calls)
}

func Test_Finish_DetachesDatabases(t *testing.T) {
	w := newWorld(map[string]bool{"dev": true})
	w.service.Status = entities.StatusDecommissioning

	_, err := usecase(w).Finish(ctx, "s1")

	require.NoError(t, err)
	require.Equal(t, []string{"detach queues", "detach databases"}, w.calls[:2])
}

func Test_Finish_StillDeletesWhenGrafanaFails(t *testing.T) {
	w := newWorld(map[string]bool{"dev": true})
	w.service.Status = entities.StatusDecommissioning
	w.dashErr = errors.New("grafana answered 500")

	deleted, err := usecase(w).Finish(ctx, "s1")

	require.NoError(t, err)
	require.True(t, deleted)
	require.Equal(t, finishCalls, w.calls)
}

func Test_Handle_FinishesWhenTheLastRemovalSucceeds(t *testing.T) {
	w := newWorld(map[string]bool{"dev": true})
	w.service.Status = entities.StatusDecommissioning
	u := usecase(w)

	require.NoError(t, u.Handle(ctx, events.Event{Action: "deploy.succeeded", TargetID: "s1", Details: map[string]any{"trigger": "manual"}}))
	require.NoError(t, u.Handle(ctx, events.Event{Action: "deploy.failed", TargetID: "s1", Details: map[string]any{"trigger": "decommission"}}))
	require.Empty(t, w.calls)
	require.NoError(t, u.Handle(ctx, events.Event{Action: "deploy.succeeded", TargetID: "s1", Details: map[string]any{"trigger": "decommission"}}))

	require.Equal(t, finishCalls, w.calls)
}
