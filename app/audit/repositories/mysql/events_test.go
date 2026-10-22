package mysql_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/audit/entities"
	"github.com/klzn-labs/kalzone/app/audit/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/klzn-labs/kalzone/pkg/page"
	"github.com/stretchr/testify/require"
)

func event(id string, second int, team, target, env string) entities.Event {
	return entities.Event{
		ID: id, At: time.Date(2026, 10, 7, 0, 0, second, 0, time.UTC), ActorID: "u1", ActorLogin: "ana",
		Action: "x", TargetKind: "service", TargetID: target, TeamID: team, Environment: env,
		Details: map[string]any{"k": "v"},
	}
}

func ids(events []entities.Event) []string {
	result := []string{}
	for _, e := range events {
		result = append(result, e.ID)
	}
	return result
}

func Test_List_FiltersAndOrdersNewestFirst(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewEventRepository(testdb.Open(t))
	require.NoError(t, repo.Insert(ctx, event("e1", 1, "t1", "s1", "dev")))
	require.NoError(t, repo.Insert(ctx, event("e2", 2, "t1", "s2", "prod")))
	require.NoError(t, repo.Insert(ctx, event("e3", 3, "t2", "s3", "prod")))
	before := time.Date(2026, 10, 7, 0, 0, 3, 0, time.UTC)

	all, err := repo.List(ctx, entities.Filter{AllTeams: true, Limit: 10})
	require.NoError(t, err)
	team, _ := repo.List(ctx, entities.Filter{AllTeams: true, TeamID: "t1", Limit: 10})
	prod, _ := repo.List(ctx, entities.Filter{AllTeams: true, Environment: "prod", Limit: 10})
	older, _ := repo.List(ctx, entities.Filter{AllTeams: true, Before: &before, Limit: 1})

	require.Equal(t, []string{"e3", "e2", "e1"}, ids(all))
	require.Equal(t, map[string]any{"k": "v"}, all[0].Details)
	require.Equal(t, []string{"e2", "e1"}, ids(team))
	require.Equal(t, []string{"e3", "e2"}, ids(prod))
	require.Equal(t, []string{"e2"}, ids(older))
}

func Test_List_FiltersByTokenAndByTeams(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewEventRepository(testdb.Open(t))
	viaToken := event("e1", 1, "t1", "s1", "dev")
	viaToken.TokenID, viaToken.TokenName = "k1", "claude-local"
	require.NoError(t, repo.Insert(ctx, viaToken))
	require.NoError(t, repo.Insert(ctx, event("e2", 2, "t2", "s2", "dev")))
	require.NoError(t, repo.Insert(ctx, event("e3", 3, "", "u1", "")))

	byToken, err := repo.List(ctx, entities.Filter{AllTeams: true, TokenID: "k1", Limit: 10})
	require.NoError(t, err)
	byTeams, _ := repo.List(ctx, entities.Filter{TeamIDs: []string{"t1", "t2"}, Limit: 10})

	require.Equal(t, []string{"e1"}, ids(byToken))
	require.Equal(t, "claude-local", byToken[0].TokenName)
	require.Equal(t, []string{"e2", "e1"}, ids(byTeams))
}

func Test_List_ScopedToTeamsAndOwnEvents(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewEventRepository(testdb.Open(t))
	other := event("e2", 2, "", "u2", "")
	other.ActorID = "u2"
	require.NoError(t, repo.Insert(ctx, event("e1", 1, "t1", "s1", "dev")))
	require.NoError(t, repo.Insert(ctx, other))
	require.NoError(t, repo.Insert(ctx, event("e3", 3, "", "u1", "")))
	require.NoError(t, repo.Insert(ctx, event("e4", 4, "t2", "s2", "dev")))
	elsewhere := event("e5", 5, "t3", "s3", "dev")
	elsewhere.ActorID = "u2"
	require.NoError(t, repo.Insert(ctx, elsewhere))

	none, err := repo.List(ctx, entities.Filter{Limit: 10})
	require.NoError(t, err)
	mine, _ := repo.List(ctx, entities.Filter{ViewerID: "u2", Limit: 10})
	scoped, _ := repo.List(ctx, entities.Filter{TeamIDs: []string{"t2"}, ViewerID: "u2", Limit: 10})

	require.Empty(t, none)
	require.Equal(t, []string{"e2"}, ids(mine))
	require.Equal(t, []string{"e4", "e2"}, ids(scoped))
}

func Test_List_ContinuesAfterEventsOfTheSameInstant(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewEventRepository(testdb.Open(t))
	for _, id := range []string{"e1", "e2", "e3"} {
		require.NoError(t, repo.Insert(ctx, event(id, 1, "t1", "s1", "dev")))
	}
	at := time.Date(2026, 10, 7, 0, 0, 1, 0, time.UTC)

	first, err := repo.List(ctx, entities.Filter{AllTeams: true, Limit: 2})
	require.NoError(t, err)
	rest, err := repo.List(ctx, entities.Filter{AllTeams: true, Before: &at, BeforeID: first[1].ID, Limit: 2})
	require.NoError(t, err)

	require.Equal(t, []string{"e3", "e2"}, ids(first))
	require.Equal(t, []string{"e1"}, ids(rest))
}

func Test_List_PagesAcrossEventsStoredInTheSameMillisecond(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewEventRepository(testdb.Open(t))
	base := time.Date(2026, 10, 7, 0, 0, 1, 123000000, time.UTC)
	offsets := map[string]time.Duration{"e0": -time.Millisecond, "e1": 400 * time.Microsecond, "e2": 100 * time.Microsecond, "e3": 300 * time.Microsecond}
	for id, offset := range offsets {
		e := event(id, 0, "t1", "s1", "dev")
		e.At = base.Add(offset)
		require.NoError(t, repo.Insert(ctx, e))
	}

	first, err := repo.List(ctx, entities.Filter{AllTeams: true, Limit: 2})
	require.NoError(t, err)
	next, err := page.Parse("2", page.Encode(page.TimeKey(first[1].At, first[1].ID)))
	require.NoError(t, err)
	before, beforeID, err := next.Time()
	require.NoError(t, err)
	rest, err := repo.List(ctx, entities.Filter{AllTeams: true, Before: before, BeforeID: beforeID, Limit: 3})
	require.NoError(t, err)

	require.Equal(t, []string{"e3", "e2"}, ids(first))
	require.Equal(t, []string{"e1", "e0"}, ids(rest))
}

func Test_List_KeysetPagesStayWithinTheTeams(t *testing.T) {
	ctx := context.Background()
	repo := mysql.NewEventRepository(testdb.Open(t))
	for i, team := range []string{"t1", "t2", "t1", "t2", "t1"} {
		require.NoError(t, repo.Insert(ctx, event("e"+strconv.Itoa(i), 1, team, "s1", "dev")))
	}
	at := time.Date(2026, 10, 7, 0, 0, 1, 0, time.UTC)
	teams := []string{"t1", "t3"}

	first, err := repo.List(ctx, entities.Filter{TeamIDs: teams, Limit: 2})
	require.NoError(t, err)
	rest, err := repo.List(ctx, entities.Filter{TeamIDs: teams, Before: &at, BeforeID: first[1].ID, Limit: 5})
	require.NoError(t, err)

	require.Equal(t, []string{"e4", "e2"}, ids(first))
	require.Equal(t, []string{"e0"}, ids(rest))
}
