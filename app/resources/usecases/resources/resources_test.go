package resources_test

import (
	"context"
	"testing"

	"github.com/klzn-labs/kalzone/app/resources/entities"
	"github.com/klzn-labs/kalzone/app/resources/usecases/resources"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type source struct{ items []entities.Resource }

func (s source) Resources(_ context.Context, _ actor.Actor, _ string) ([]entities.Resource, error) {
	return s.items, nil
}

func usecase() resources.ResourcesUsecase {
	return resources.NewResourcesUsecase(resources.Sources{
		entities.KindQueue: source{items: []entities.Resource{
			{Kind: entities.KindQueue, Name: "b", Environment: "dev"},
			{Kind: entities.KindQueue, Name: "a", Environment: "prod"},
		}},
		entities.KindDatabase: source{items: []entities.Resource{
			{Kind: entities.KindDatabase, Name: "z", Environment: "dev"},
			{Kind: entities.KindDatabase, Name: "a", Environment: "dev"},
		}},
	})
}

func names(list []entities.Resource) []string {
	result := make([]string, 0, len(list))
	for _, r := range list {
		result = append(result, string(r.Kind)+"/"+r.Environment+"/"+r.Name)
	}
	return result
}

func Test_ListFor_MergesSourcesSorted(t *testing.T) {
	list, err := usecase().ListFor(ctx, actor.Actor{}, "", "")

	require.NoError(t, err)
	require.Equal(t, []string{"database/dev/a", "database/dev/z", "queue/dev/b", "queue/prod/a"}, names(list))
}

func Test_ListFor_OnlyTheAskedKind(t *testing.T) {
	list, err := usecase().ListFor(ctx, actor.Actor{}, entities.KindQueue, "")

	require.NoError(t, err)
	require.Equal(t, []string{"queue/dev/b", "queue/prod/a"}, names(list))
}

func Test_ListFor_RejectsUnknownKind(t *testing.T) {
	_, err := usecase().ListFor(ctx, actor.Actor{}, "nope", "")

	var invalid *apperror.AppError
	require.ErrorAs(t, err, &invalid)
	require.Equal(t, "kind", invalid.Field)
	require.Equal(t, "send one of queue, database, cron", invalid.Hint)
}

func Test_ListFor_KnownKindWithoutSourceIsEmpty(t *testing.T) {
	only := resources.NewResourcesUsecase(resources.Sources{entities.KindQueue: source{}})

	list, err := only.ListFor(ctx, actor.Actor{}, entities.KindDatabase, "")

	require.NoError(t, err)
	require.Empty(t, list)
	require.NotNil(t, list)
}
