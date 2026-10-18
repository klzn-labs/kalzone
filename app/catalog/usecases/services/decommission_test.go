package services_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_MarkDecommissioning_NeedsDeleteRightsAndHappensOnce(t *testing.T) {
	s, c, p, usecase := setup(entities.Service{ID: "s1", Slug: "api", Name: "API", OwnerTeamID: "t1"})

	first, err := usecase.MarkDecommissioning(ctx, ana, "api")
	again, againErr := usecase.MarkDecommissioning(ctx, ana, "api")

	require.NoError(t, err)
	require.NoError(t, againErr)
	require.True(t, first.Decommissioning())
	require.True(t, again.Decommissioning())
	require.True(t, s.bySlug["api"].Decommissioning())
	require.Equal(t, []authz.Action{authz.DeleteService, authz.DeleteService}, c.actions)
	require.Len(t, p.events, 1)
	require.Equal(t, "service.decommissioning", p.events[0].Action)
}

func Test_ADecommissioningServiceCannotBeEditedOrTransferred(t *testing.T) {
	_, _, _, usecase := setup(entities.Service{ID: "s1", Slug: "api", Name: "API", OwnerTeamID: "t1", Status: entities.StatusDecommissioning})
	name := "New"

	_, edit := usecase.Update(ctx, ana, "api", entities.ServiceChanges{Name: &name})
	_, transfer := usecase.Transfer(ctx, ana, "api", "reviews")

	require.ErrorIs(t, edit, apperror.Conflict(""))
	require.ErrorIs(t, transfer, apperror.Conflict(""))
}

func Test_MarkDeleted_HidesTheServiceAndNamesTheActor(t *testing.T) {
	s, _, p, usecase := setup(entities.Service{ID: "s1", Slug: "api", Name: "API", OwnerTeamID: "t1", Status: entities.StatusDecommissioning})

	require.NoError(t, usecase.MarkDeleted(ctx, actor.Actor{Login: "decommission"}, "s1"))

	require.NotNil(t, s.bySlug["api"].DeletedAt)
	require.Equal(t, "service.deleted", p.events[0].Action)
	require.Equal(t, "decommission", p.events[0].ActorLogin)
}
