package search_test

import (
	"context"
	"testing"

	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/app/envconfig/usecases/search"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type store []entities.Match

func (s store) Search(_ context.Context, key, environment string) ([]entities.Match, error) {
	result := []entities.Match{}
	for _, m := range s {
		if environment == "" || m.Environment == environment {
			result = append(result, m)
		}
	}
	return result, nil
}

type directory struct{}

func (directory) Services(context.Context) ([]entities.ServiceRef, error) {
	return []entities.ServiceRef{
		{ID: "s1", Slug: "worker", OwnerTeamID: "t1"},
		{ID: "s2", Slug: "api", OwnerTeamID: "t1"},
		{ID: "s3", Slug: "billing", OwnerTeamID: "t2"},
	}, nil
}

func (directory) TeamID(_ context.Context, slug string) (string, error) {
	ids := map[string]string{"places": "t1", "money": "t2"}
	if id, found := ids[slug]; found {
		return id, nil
	}
	return "", apperror.NotFound("team not found")
}

type environments struct{}

func (environments) EnvironmentExists(_ context.Context, slug string) error {
	if slug != "dev" && slug != "prod" {
		return apperror.NotFound("environment not found")
	}
	return nil
}

var matches = store{
	{ServiceID: "s1", Environment: "prod", Revision: 7, Kind: entities.MatchSecret},
	{ServiceID: "s2", Environment: "prod", Revision: 12, Kind: entities.MatchEnv, Value: "info"},
	{ServiceID: "s3", Environment: "dev", Revision: 3, Kind: entities.MatchEnv, Value: "debug"},
	{ServiceID: "gone", Environment: "dev", Revision: 1, Kind: entities.MatchEnv, Value: "x"},
}

var placesToken = actor.Actor{UserID: "ana", Token: actor.Token{ID: "k1", TeamIDs: []string{"t1"}}}

func usecase() search.SearchUsecase {
	return search.NewSearchUsecase(matches, directory{}, environments{})
}

func Test_Find_OrdersMatchesByServiceAndHidesOtherTeams(t *testing.T) {
	found, err := usecase().Find(ctx, placesToken, search.Query{Key: "LOG_LEVEL"})

	require.NoError(t, err)
	require.Equal(t, []entities.Match{
		{ServiceID: "s2", ServiceSlug: "api", Environment: "prod", Revision: 12, Kind: entities.MatchEnv, Value: "info"},
		{ServiceID: "s1", ServiceSlug: "worker", Environment: "prod", Revision: 7, Kind: entities.MatchSecret},
	}, found)
}

func Test_Find_NarrowsByTeamAndEnvironment(t *testing.T) {
	session := actor.Actor{UserID: "ana"}

	money, err := usecase().Find(ctx, session, search.Query{Key: "LOG_LEVEL", Team: "money"})
	require.NoError(t, err)
	prod, err := usecase().Find(ctx, session, search.Query{Key: "LOG_LEVEL", Environment: "prod"})
	require.NoError(t, err)

	require.Len(t, money, 1)
	require.Equal(t, "billing", money[0].ServiceSlug)
	require.Len(t, prod, 2)
}

func Test_Find_RefusesBadKeysUnknownEnvironmentsAndUngrantedTeams(t *testing.T) {
	_, missing := usecase().Find(ctx, placesToken, search.Query{})
	_, path := usecase().Find(ctx, placesToken, search.Query{Key: `A"].x`})
	_, env := usecase().Find(ctx, placesToken, search.Query{Key: "A", Environment: "qa"})
	_, team := usecase().Find(ctx, placesToken, search.Query{Key: "A", Team: "money"})

	require.Equal(t, "key", missing.(*apperror.AppError).Field)
	require.Equal(t, "key", path.(*apperror.AppError).Field)
	require.ErrorIs(t, env, apperror.NotFound(""))
	require.ErrorIs(t, team, apperror.TeamNotGranted(""))
}

func Test_Find_NeverShowsASecretValue(t *testing.T) {
	leaky := store{{ServiceID: "s1", Environment: "prod", Revision: 7, Kind: entities.MatchSecret, Value: "hunter2"}}

	found, err := search.NewSearchUsecase(leaky, directory{}, environments{}).Find(ctx, placesToken, search.Query{Key: "TOKEN"})

	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Empty(t, found[0].Value)
	require.Equal(t, entities.MatchSecret, found[0].Kind)
}
