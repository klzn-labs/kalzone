package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/app/deploy/usecases/fakestore"
	"github.com/adrianozp/platform/app/deploy/usecases/runtime"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var (
	ctx      = context.Background()
	ana      = actor.Actor{UserID: "ana", Login: "ana"}
	outsider = actor.Actor{UserID: "zed", Token: actor.Token{ID: "k1", TeamIDs: []string{"t9"}}}
)

type meter struct {
	cpu, memory int
	ok          bool
	asked       string
}

func (m *meter) Usage(_ context.Context, container string) (int, int, bool) {
	m.asked = container
	return m.cpu, m.memory, m.ok
}

func setup(m *meter, observed time.Time) runtime.RuntimeUsecase {
	store := fakestore.New()
	store.Runtimes["s1/dev"] = entities.RuntimeStatus{
		ServiceID: "s1", Environment: "dev", Container: "api-dev-blue", Found: true, State: "running",
		MemoryLimitMB: 128, CPULimitMillis: 250, ObservedAt: observed,
	}
	return runtime.NewRuntimeUsecase(store, fakestore.Services{}, m)
}

func Test_StatusFor_MergesUsageWithTheLimits(t *testing.T) {
	m := &meter{cpu: 3, memory: 12, ok: true}

	view, err := setup(m, time.Now()).StatusFor(ctx, ana, "api", "dev")

	require.NoError(t, err)
	require.Equal(t, "api-dev-blue", m.asked)
	require.Equal(t, &entities.Usage{CPUMillis: 3, CPULimitMillis: 250, MemoryMB: 12, MemoryLimitMB: 128}, view.Usage)
	require.False(t, view.Stale)
	require.Equal(t, "running", view.Status.State)
}

func Test_StatusFor_LeavesUsageOutWhenMetricsFail(t *testing.T) {
	view, err := setup(&meter{}, time.Now()).StatusFor(ctx, ana, "api", "dev")

	require.NoError(t, err)
	require.Nil(t, view.Usage)
}

func Test_StatusFor_SaysWhenTheAgentStoppedReporting(t *testing.T) {
	view, err := setup(&meter{}, time.Now().Add(-2*time.Minute)).StatusFor(ctx, ana, "api", "dev")

	require.NoError(t, err)
	require.True(t, view.Stale)
}

func Test_StatusFor_RefusesWhatTheCallerCannotSeeOrWhatIsNotReported(t *testing.T) {
	u := setup(&meter{}, time.Now())

	_, hidden := u.StatusFor(ctx, outsider, "api", "dev")
	_, none := u.StatusFor(ctx, ana, "api", "prod")
	_, missing := u.StatusFor(ctx, ana, "", "dev")

	require.ErrorIs(t, hidden, apperror.NotFound(""))
	require.ErrorIs(t, none, apperror.NotFound(""))
	require.ErrorIs(t, missing, apperror.Invalid(""))
}
