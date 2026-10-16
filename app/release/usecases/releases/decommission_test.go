package releases_test

import (
	"testing"

	"github.com/adrianozp/platform/app/release/entities"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_ADecommissioningServiceGetsNoNewReleases(t *testing.T) {
	f := newFixture(t, false)

	_, requested := f.usecase.Request(ctx, ana, "retired", "main")
	_, registered := f.usecase.Register(ctx, ana, "retired", "v1")
	pushed, err := f.usecase.OnPush(ctx, entities.PushEvent{Repository: "o/retired", Ref: "refs/heads/main", DefaultBranch: "main", After: sha})

	require.ErrorIs(t, requested, apperror.Conflict(""))
	require.ErrorIs(t, registered, apperror.Conflict(""))
	require.NoError(t, err)
	require.Empty(t, pushed)
	require.Empty(t, f.builder.requests)
}

func Test_FailUnfinished_FailsTheServicesOpenBuildsOnly(t *testing.T) {
	f := newFixture(t, false)
	apiBuild, _ := f.usecase.Request(ctx, ana, "api", "main")
	webBuild, _ := f.usecase.Request(ctx, ana, "web", "main")

	require.NoError(t, f.usecase.FailUnfinished(ctx, ana, "s1"))

	failed, _ := f.store.FindByID(ctx, apiBuild.ID)
	open, _ := f.store.FindByID(ctx, webBuild.ID)
	require.Equal(t, entities.StatusFailed, failed.Status)
	require.Equal(t, "the service is being deleted", failed.Failure)
	require.Equal(t, entities.StatusBuilding, open.Status)
}
