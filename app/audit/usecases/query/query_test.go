package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/audit/entities"
	"github.com/klzn-labs/kalzone/app/audit/usecases/query"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/page"
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

func Test_ListFor_ScopesAMemberToTheirTeamsAndOwnEvents(t *testing.T) {
	c := &capture{}
	usecase := query.NewQueryUsecase(c)

	_, err := usecase.ListFor(context.Background(), actor.Actor{UserID: "u1", Teams: []string{"t1"}}, entities.Filter{Environment: "dev"})

	require.NoError(t, err)
	require.Equal(t, []string{"t1"}, c.last.TeamIDs)
	require.Equal(t, "u1", c.last.ViewerID)
	require.False(t, c.last.AllTeams)
	require.Equal(t, "dev", c.last.Environment)
}

func Test_ListFor_LimitsATokenToItsTeamsTheOwnerSees(t *testing.T) {
	c := &capture{}
	usecase := query.NewQueryUsecase(c)
	agent := actor.Actor{UserID: "u1", Teams: []string{"t1", "t2"}, Token: actor.Token{ID: "tok", TeamIDs: []string{"t2", "t3"}}}

	_, err := usecase.ListFor(context.Background(), agent, entities.Filter{})

	require.NoError(t, err)
	require.Equal(t, []string{"t2"}, c.last.TeamIDs)
	require.False(t, c.last.AllTeams)
}

func Test_ListFor_AnAdminSessionSeesAllTeams(t *testing.T) {
	c := &capture{}
	usecase := query.NewQueryUsecase(c)

	_, err := usecase.ListFor(context.Background(), actor.Actor{UserID: "u1", Admin: true}, entities.Filter{})

	require.NoError(t, err)
	require.True(t, c.last.AllTeams)
}

type events []entities.Event

func (e events) List(_ context.Context, f entities.Filter) ([]entities.Event, error) {
	result := []entities.Event{}
	for _, event := range e {
		if f.Before == nil || event.At.Before(*f.Before) || (event.At.Equal(*f.Before) && event.ID < f.BeforeID) {
			result = append(result, event)
		}
	}
	return result[:min(f.Limit, len(result))], nil
}

func Test_PageFor_ContinuesFromTheCursor(t *testing.T) {
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	usecase := query.NewQueryUsecase(events{{ID: "e3", At: at}, {ID: "e2", At: at}, {ID: "e1", At: at.Add(-time.Second)}})

	first, err := usecase.PageFor(context.Background(), actor.Actor{UserID: "u1"}, entities.Filter{}, page.First(2))
	require.NoError(t, err)
	next, _ := page.Parse("2", first.Next)
	second, err := usecase.PageFor(context.Background(), actor.Actor{UserID: "u1"}, entities.Filter{}, next)
	require.NoError(t, err)
	_, garbage := usecase.PageFor(context.Background(), actor.Actor{UserID: "u1"}, entities.Filter{}, page.Request{Limit: 2, After: "x"})

	require.Equal(t, "e2", first.Items[1].ID)
	require.Len(t, second.Items, 1)
	require.Equal(t, "e1", second.Items[0].ID)
	require.Equal(t, "cursor", garbage.(*apperror.AppError).Field)
}
