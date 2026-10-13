package deployments_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/app/deploy/usecases/deployments"
	"github.com/adrianozp/platform/app/deploy/usecases/fakestore"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/page"
	"github.com/stretchr/testify/require"
)

var outsider = actor.Actor{UserID: "zed", Token: actor.Token{ID: "k1", TeamIDs: []string{"t9"}}}

func logged(store *fakestore.Store, status entities.Status, count int) entities.Deployment {
	d := entities.Deployment{ID: "d1", ServiceID: fakestore.API.ID, Environment: "dev", Status: status, CreatedAt: time.Now()}
	store.ByID[d.ID] = d
	for i := 1; i <= count; i++ {
		store.Lines[d.ID] = append(store.Lines[d.ID], fmt.Sprintf("line %d", i))
	}
	return d
}

func messages(events []entities.Event) []string {
	result := []string{}
	for _, e := range events {
		result = append(result, e.Message)
	}
	return result
}

func Test_PageFor_PagesTheHistoryNewestFirst(t *testing.T) {
	store, usecase := setup("")
	for _, release := range []string{"r1", "r2", "r1"} {
		_, err := usecase.Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "dev", ReleaseID: release})
		require.NoError(t, err)
	}

	first, err := usecase.PageFor(ctx, ana, "api", "dev", page.First(2))
	require.NoError(t, err)
	next, _ := page.Parse("2", first.Next)
	second, err := usecase.PageFor(ctx, ana, "api", "dev", next)
	require.NoError(t, err)

	require.Equal(t, []int{3, 2}, []int{first.Items[0].Number, first.Items[1].Number})
	require.Equal(t, 1, second.Items[0].Number)
	require.Empty(t, second.Next)
	require.Len(t, store.ByID, 3)
}

func Test_PageFor_NamesTheMissingQueryParameter(t *testing.T) {
	_, usecase := setup("")

	_, noService := usecase.PageFor(ctx, ana, "", "dev", page.First(5))
	_, noEnv := usecase.PageFor(ctx, ana, "api", "", page.First(5))
	_, hidden := usecase.PageFor(ctx, outsider, "api", "dev", page.First(5))

	require.Equal(t, "service", noService.(*apperror.AppError).Field)
	require.Equal(t, "env", noEnv.(*apperror.AppError).Field)
	require.ErrorIs(t, hidden, apperror.NotFound(""))
}

func Test_SummaryFor_CountsEveryLineAndKeepsTheTail(t *testing.T) {
	store, usecase := setup("")
	logged(store, entities.StatusSucceeded, 30)

	_, summary, err := usecase.SummaryFor(ctx, ana, "d1")
	_, _, hidden := usecase.SummaryFor(ctx, outsider, "d1")

	require.NoError(t, err)
	require.Equal(t, 30, summary.Total)
	require.Len(t, summary.Tail, 20)
	require.Equal(t, "line 30", summary.Tail[19].Message)
	require.ErrorIs(t, hidden, apperror.NotFound(""))
}

func Test_EventsFor_ReadsForwardAndTails(t *testing.T) {
	store, usecase := setup("")
	logged(store, entities.StatusSucceeded, 5)

	first, err := usecase.EventsFor(ctx, ana, "d1", page.First(3), "")
	require.NoError(t, err)
	next, _ := page.Parse("3", first.Next)
	rest, err := usecase.EventsFor(ctx, ana, "d1", next, "")
	require.NoError(t, err)
	tail, err := usecase.EventsFor(ctx, ana, "d1", page.First(20), "2")
	require.NoError(t, err)
	_, both := usecase.EventsFor(ctx, ana, "d1", next, "2")
	_, tooMany := usecase.EventsFor(ctx, ana, "d1", page.First(20), "500")

	require.Equal(t, []string{"line 1", "line 2", "line 3"}, messages(first.Items))
	require.Equal(t, []string{"line 4", "line 5"}, messages(rest.Items))
	require.Empty(t, rest.Next)
	require.Equal(t, []string{"line 4", "line 5"}, messages(tail.Items))
	require.Empty(t, tail.Next)
	require.Equal(t, "tail", both.(*apperror.AppError).Field)
	require.Equal(t, "tail", tooMany.(*apperror.AppError).Field)
	require.Equal(t, "tail must be a whole number from 1 to 100", tooMany.Error())
}

func Test_EventsFor_KeepsACursorWhileTheDeploymentRuns(t *testing.T) {
	store, usecase := setup("")
	logged(store, entities.StatusInProgress, 2)

	all, err := usecase.EventsFor(ctx, ana, "d1", page.First(20), "")
	require.NoError(t, err)
	next, _ := page.Parse("", all.Next)
	nothingYet, err := usecase.EventsFor(ctx, ana, "d1", next, "")
	require.NoError(t, err)
	store.Lines["d1"] = append(store.Lines["d1"], "line 3")
	again, _ := page.Parse("", nothingYet.Next)
	more, err := usecase.EventsFor(ctx, ana, "d1", again, "")

	require.NoError(t, err)
	require.Len(t, all.Items, 2)
	require.Equal(t, all.Next, nothingYet.Next)
	require.Equal(t, []string{"line 3"}, messages(more.Items))
}

func Test_PageFor_AnswersAHiddenServiceLikeAMissingOne(t *testing.T) {
	_, usecase := setup("")

	_, hidden := usecase.PageFor(ctx, outsider, "api", "dev", page.First(5))
	_, missing := usecase.PageFor(ctx, ana, "nope", "dev", page.First(5))

	want := apperror.AppError{Code: apperror.CodeNotFound, Message: "service not found", Hint: "list services with GET /api/v1/services"}
	require.Equal(t, want, *hidden.(*apperror.AppError))
	require.Equal(t, want, *missing.(*apperror.AppError))
}

func Test_ReadsOfAHiddenDeploymentAnswerLikeAMissingOne(t *testing.T) {
	store, usecase := setup("")
	logged(store, entities.StatusSucceeded, 1)

	_, _, hiddenSummary := usecase.SummaryFor(ctx, outsider, "d1")
	_, hiddenEvents := usecase.EventsFor(ctx, outsider, "d1", page.First(5), "")
	_, _, missing := usecase.SummaryFor(ctx, ana, "nope")

	want := apperror.AppError{Code: apperror.CodeNotFound, Message: "deployment not found", Hint: "list deployments with GET /api/v1/deployments?service=<slug>&env=<env>"}
	require.Equal(t, want, *hiddenSummary.(*apperror.AppError))
	require.Equal(t, want, *hiddenEvents.(*apperror.AppError))
	require.Equal(t, want, *missing.(*apperror.AppError))
}
