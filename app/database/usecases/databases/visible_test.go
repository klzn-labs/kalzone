package databases_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var reviewsToken = actor.Actor{UserID: "u-ana", Teams: []string{"t-plateful", "t-reviews"}, Token: actor.Token{ID: "tk", TeamIDs: []string{"t-reviews"}}}

func Test_GetFor_HidesOtherTeams(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())

	_, err := f.open().GetFor(ctx, reviewsToken, "dev", "shop")
	require.Equal(t, apperror.CodeNotFound, apperror.CodeOf(err))
	got, err := f.open().GetFor(ctx, ana, "dev", "shop")
	require.NoError(t, err)
	require.Equal(t, "shop", got.Name)
}

func Test_ListFor_FiltersByVisibilityAndTeam(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())
	f.created(t, databases.Draft{Name: "reviews", Environment: "dev", Team: "reviews"})

	visible, err := f.open().ListFor(ctx, reviewsToken, "dev", "")
	require.NoError(t, err)
	byTeam, err := f.open().ListFor(ctx, ana, "", "plateful")
	require.NoError(t, err)

	require.Len(t, visible, 1)
	require.Equal(t, "reviews", visible[0].Name)
	require.Len(t, byTeam, 1)
	require.Equal(t, "shop", byTeam[0].Name)
}

func Test_AttachmentsFor_MarksDraining(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())
	f.attached(t, "dev", "shop", "shop", "worker")
	_, _ = f.open().Detach(ctx, ana, "dev", "shop", "worker")
	f.running["s-worker"] = true

	views, err := f.open().AttachmentsFor(ctx, ana, "dev", "shop")

	require.NoError(t, err)
	require.Len(t, views, 2)
	require.Equal(t, "shop", views[0].Service)
	require.False(t, views[0].Draining)
	require.Equal(t, "worker", views[1].Service)
	require.True(t, views[1].Draining)
	require.NotNil(t, views[1].DetachedAt)
}
