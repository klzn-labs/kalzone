package teams_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/app/team/usecases/teams"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func twoTeams() teams.TeamsUsecase {
	s := &store{bySlug: map[string]entities.Team{"places": {ID: "t1", Slug: "places"}, "reviews": {ID: "t2", Slug: "reviews"}}}
	return teams.NewTeamsUsecase(s, services(0), allow{}, &published{})
}

func Test_ListFor_ShowsAMemberOnlyTheirTeams(t *testing.T) {
	usecase := twoTeams()

	mine, err := usecase.ListFor(ctx, actor.Actor{UserID: "u1", Teams: []string{"t2"}})
	all, _ := usecase.ListFor(ctx, admin)

	require.NoError(t, err)
	require.Equal(t, []entities.Team{{ID: "t2", Slug: "reviews"}}, mine)
	require.Len(t, all, 2)
}

func Test_GetFor_HidesATeamFromAnOutsider(t *testing.T) {
	usecase := twoTeams()

	_, hidden := usecase.GetFor(ctx, actor.Actor{UserID: "u1", Teams: []string{"t2"}}, "places")
	team, err := usecase.GetFor(ctx, actor.Actor{UserID: "u1", Teams: []string{"t2"}}, "reviews")

	require.ErrorIs(t, hidden, apperror.NotFound(""))
	require.NoError(t, err)
	require.Equal(t, "t2", team.ID)
}
