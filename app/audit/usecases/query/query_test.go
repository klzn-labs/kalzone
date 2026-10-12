package query_test

import (
	"context"
	"testing"

	"github.com/adrianozp/platform/app/audit/entities"
	"github.com/adrianozp/platform/app/audit/usecases/query"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/stretchr/testify/require"
)

type store struct{ limits []int }

func (s *store) List(_ context.Context, f entities.Filter) ([]entities.Event, error) {
	s.limits = append(s.limits, f.Limit)
	return nil, nil
}

func Test_List_ClampsTheLimit(t *testing.T) {
	s := &store{}
	usecase := query.NewQueryUsecase(s)

	for _, limit := range []int{0, 10, 1000} {
		_, err := usecase.List(context.Background(), entities.Filter{Limit: limit})
		require.NoError(t, err)
	}

	require.Equal(t, []int{50, 10, 200}, s.limits)
}

type capture struct {
	last  entities.Filter
	calls int
}

func (c *capture) List(_ context.Context, f entities.Filter) ([]entities.Event, error) {
	c.last = f
	c.calls++
	return nil, nil
}

func Test_ListFor_LimitsATokenToItsTeams(t *testing.T) {
	c := &capture{}
	usecase := query.NewQueryUsecase(c)
	agent := actor.Actor{UserID: "u1", Token: actor.Token{ID: "tok", TeamIDs: []string{"t1"}}}

	_, err := usecase.ListFor(context.Background(), agent, entities.Filter{})
	require.NoError(t, err)
	viaToken := c.last
	_, _ = usecase.ListFor(context.Background(), actor.Actor{UserID: "u1"}, entities.Filter{})

	require.Equal(t, []string{"t1"}, viaToken.TeamIDs)
	require.Empty(t, c.last.TeamIDs)
}

func Test_ListFor_ReturnsNothingForATokenWithoutTeams(t *testing.T) {
	c := &capture{calls: 0}
	usecase := query.NewQueryUsecase(c)
	agent := actor.Actor{UserID: "u1", Token: actor.Token{ID: "tok"}}

	events, err := usecase.ListFor(context.Background(), agent, entities.Filter{})

	require.NoError(t, err)
	require.Empty(t, events)
	require.Zero(t, c.calls)
}

func Test_ListFor_PassesTheTokenTeamsToTheStore(t *testing.T) {
	c := &capture{}
	usecase := query.NewQueryUsecase(c)
	agent := actor.Actor{UserID: "u1", Token: actor.Token{ID: "tok", TeamIDs: []string{"t1", "t2"}}}

	_, err := usecase.ListFor(context.Background(), agent, entities.Filter{Environment: "dev"})

	require.NoError(t, err)
	require.Equal(t, []string{"t1", "t2"}, c.last.TeamIDs)
	require.Equal(t, "dev", c.last.Environment)
	require.Equal(t, 1, c.calls)
}
