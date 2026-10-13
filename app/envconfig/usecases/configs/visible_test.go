package configs_test

import (
	"testing"

	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/page"
	"github.com/stretchr/testify/require"
)

func Test_RevisionsFor_PagesNewestFirst(t *testing.T) {
	f := newFixture(t)
	for _, value := range []string{"1", "2", "3"} {
		_, err := f.usecase.Revise(ctx, ana, "api", "dev", entities.ChangeSet{SetEnv: map[string]string{"A": value}})
		require.NoError(t, err)
	}

	first, err := f.usecase.RevisionsFor(ctx, ana, "api", "dev", page.First(2))
	require.NoError(t, err)
	next, _ := page.Parse("2", first.Next)
	second, err := f.usecase.RevisionsFor(ctx, ana, "api", "dev", next)
	require.NoError(t, err)
	_, hidden := f.usecase.RevisionsFor(ctx, actor.Actor{UserID: "zed", Token: actor.Token{ID: "k1"}}, "api", "dev", page.First(2))

	require.Equal(t, []int{3, 2}, []int{first.Items[0].Number, first.Items[1].Number})
	require.Equal(t, 1, second.Items[0].Number)
	require.Empty(t, second.Next)
	require.ErrorIs(t, hidden, apperror.NotFound(""))
}
