package releases_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/page"
	"github.com/stretchr/testify/require"
)

func Test_PageFor_PagesNewestFirstWithinTheService(t *testing.T) {
	f := newFixture(t, false)
	for _, id := range []string{"r1", "r2", "w1", "r3"} {
		service := api.ID
		if id == "w1" {
			service = worker.ID
		}
		_, _ = f.store.Create(ctx, entities.Release{ID: id, ServiceID: service})
	}

	first, err := f.usecase.PageFor(ctx, ana, "api", page.First(2))
	require.NoError(t, err)
	next, err := page.Parse("2", first.Next)
	require.NoError(t, err)
	second, err := f.usecase.PageFor(ctx, ana, "api", next)
	require.NoError(t, err)

	require.Equal(t, []string{"r3", "r2"}, []string{first.Items[0].ID, first.Items[1].ID})
	require.Len(t, second.Items, 1)
	require.Equal(t, "r1", second.Items[0].ID)
	require.Empty(t, second.Next)
}

func Test_PageFor_NeedsAServiceTheCallerSees(t *testing.T) {
	f := newFixture(t, false)
	outsider := actor.Actor{UserID: "zed", Token: actor.Token{ID: "k1", TeamIDs: []string{"t9"}}}

	_, missing := f.usecase.PageFor(ctx, ana, "", page.First(5))
	_, hidden := f.usecase.PageFor(ctx, outsider, "api", page.First(5))

	require.Equal(t, "service", missing.(*apperror.AppError).Field)
	require.ErrorIs(t, hidden, apperror.NotFound(""))
}

func Test_PageFor_AnswersAHiddenServiceLikeAMissingOne(t *testing.T) {
	f := newFixture(t, false)
	outsider := actor.Actor{UserID: "zed", Token: actor.Token{ID: "k1", TeamIDs: []string{"t9"}}}

	_, hidden := f.usecase.PageFor(ctx, outsider, "api", page.First(5))
	_, missing := f.usecase.PageFor(ctx, ana, "nope", page.First(5))

	want := apperror.AppError{Code: apperror.CodeNotFound, Message: "service not found", Hint: "list services with GET /api/v1/services"}
	require.Equal(t, want, *hidden.(*apperror.AppError))
	require.Equal(t, want, *missing.(*apperror.AppError))
}

func Test_GetFor_AnswersAHiddenReleaseLikeAMissingOne(t *testing.T) {
	f := newFixture(t, false)
	_, _ = f.store.Create(ctx, entities.Release{ID: "r1", ServiceID: api.ID})
	outsider := actor.Actor{UserID: "zed", Token: actor.Token{ID: "k1", TeamIDs: []string{"t9"}}}

	_, hidden := f.usecase.GetFor(ctx, outsider, "r1")
	_, missing := f.usecase.GetFor(ctx, ana, "nope")

	require.Equal(t, *missing.(*apperror.AppError), *hidden.(*apperror.AppError))
}
