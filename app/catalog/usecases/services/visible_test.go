package services_test

import (
	"testing"

	"github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_GetFor_HidesServicesOfTeamsTheTokenLacks(t *testing.T) {
	_, _, _, usecase := setup(entities.Service{ID: "s1", Slug: "api", OwnerTeamID: "t1"})
	agent := ana
	agent.Token = actor.Token{ID: "tok", TeamIDs: []string{"t2"}}

	_, hidden := usecase.GetFor(ctx, agent, "api")
	found, err := usecase.GetFor(ctx, ana, "api")

	require.ErrorIs(t, hidden, apperror.NotFound(""))
	require.NoError(t, err)
	require.Equal(t, "s1", found.ID)
}
