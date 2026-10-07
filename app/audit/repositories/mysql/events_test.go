package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/audit/entities"
	"github.com/adrianozp/platform/app/audit/repositories/mysql"
	"github.com/adrianozp/platform/internal/testdb"
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

	all, err := repo.List(ctx, entities.Filter{Limit: 10})
	require.NoError(t, err)
	team, _ := repo.List(ctx, entities.Filter{TeamID: "t1", Limit: 10})
	prod, _ := repo.List(ctx, entities.Filter{Environment: "prod", Limit: 10})
	older, _ := repo.List(ctx, entities.Filter{Before: &before, Limit: 1})

	require.Equal(t, []string{"e3", "e2", "e1"}, ids(all))
	require.Equal(t, map[string]any{"k": "v"}, all[0].Details)
	require.Equal(t, []string{"e2", "e1"}, ids(team))
	require.Equal(t, []string{"e3", "e2"}, ids(prod))
	require.Equal(t, []string{"e2"}, ids(older))
}
