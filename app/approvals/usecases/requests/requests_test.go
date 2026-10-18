package requests_test

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/stretchr/testify/require"
)

func Test_Approve_RunsTheChangeAsTheTokenCarryingTheApproval(t *testing.T) {
	f := newFixture()

	r, err := f.usecase().Approve(ctx, mia, "c1")

	require.NoError(t, err)
	require.Equal(t, entities.StatusExecuted, r.Status)
	require.Equal(t, "deployment #3", f.store.byID["c1"].Result)
	ran := f.executor.ran[0]
	require.Equal(t, "u-bob", ran.UserID)
	require.Equal(t, "k1", ran.Token.ID)
	require.Equal(t, actor.Approval{RequestID: "c1", ApproverID: "u-mia", ApproverLogin: "mia"}, ran.Approval)
	require.Equal(t, []string{"request.approved", "request.executed"}, f.events.actions)
}

func Test_Approve_BuildsAFreshActorForEachRequest(t *testing.T) {
	f := newFixture()
	f.store.byID["c2"] = pendingRequest("c2", "k2")

	_, first := f.usecase().Approve(ctx, mia, "c1")
	_, second := f.usecase().Approve(ctx, mia, "c2")

	require.NoError(t, first)
	require.NoError(t, second)
	require.Equal(t, []string{"k1", "k2"}, f.tokens.asked)
	require.Len(t, f.executor.ran, 2)
	require.Equal(t, "c1", f.executor.ran[0].Approval.RequestID)
	require.Equal(t, "k1", f.executor.ran[0].Token.ID)
	require.Equal(t, "c2", f.executor.ran[1].Approval.RequestID)
	require.Equal(t, "k2", f.executor.ran[1].Token.ID)
}

func Test_Approve_FinishesTheChangeWhenTheCallerGoesAway(t *testing.T) {
	f := newFixture()
	gone, cancel := context.WithCancel(ctx)
	cancel()

	r, err := f.usecase().Approve(gone, mia, "c1")

	require.NoError(t, err)
	require.Equal(t, entities.StatusExecuted, r.Status)
	require.Equal(t, []error{nil}, f.executor.ctxErr)
	require.Equal(t, entities.StatusExecuted, f.store.byID["c1"].Status)
}

func Test_Approve_RecordsAFailureWithItsCode(t *testing.T) {
	f := newFixture()
	f.executor.err = apperror.Stale("env.A changed since the request")

	r, err := f.usecase().Approve(ctx, mia, "c1")

	require.NoError(t, err)
	require.Equal(t, entities.StatusFailed, r.Status)
	require.Equal(t, "err_stale", r.ErrorCode)
	require.Equal(t, "env.A changed since the request", f.store.byID["c1"].ErrorMessage)
	require.Equal(t, []string{"request.approved", "request.failed"}, f.events.actions)
}

func Test_Approve_FailsWhenTheTokenNoLongerWorks(t *testing.T) {
	f := newFixture()
	f.tokens.err = apperror.New(apperror.CodeTokenRevoked, "this access token was revoked")

	r, err := f.usecase().Approve(ctx, mia, "c1")

	require.NoError(t, err)
	require.Equal(t, "err_token_revoked", r.ErrorCode)
	require.Empty(t, f.executor.ran)
}

func Test_Approve_OnlyByAnApproverWhileStillPending(t *testing.T) {
	f := newFixture()

	_, notApprover := f.usecase().Approve(ctx, bob, "c1")
	clock.WithTime(now.Add(entities.Lifetime))
	_, expired := f.usecase().Approve(ctx, mia, "c1")
	clock.WithTime(now)
	_, _ = f.usecase().Approve(ctx, mia, "c1")
	_, twice := f.usecase().Approve(ctx, mia, "c1")

	require.ErrorIs(t, notApprover, apperror.Forbidden(""))
	require.ErrorIs(t, expired, apperror.Conflict(""))
	require.ErrorIs(t, twice, apperror.Conflict(""))
	require.Len(t, f.executor.ran, 1)
}

func Test_Approve_LosesTheRaceWhenAnotherApproverWonAfterTheRead(t *testing.T) {
	f := newFixture()
	stale := readingStale(f.store)
	_, won := f.usecase().Approve(ctx, mia, "c1")

	_, lost := f.usecaseOver(stale).Approve(ctx, mia, "c1")

	require.NoError(t, won)
	require.ErrorIs(t, lost, apperror.Conflict(""))
	require.Len(t, f.executor.ran, 1)
	require.Equal(t, entities.StatusExecuted, f.store.byID["c1"].Status)
	require.Equal(t, []string{"request.approved", "request.executed"}, f.events.actions)
}

func Test_Reject_KeepsTheReason(t *testing.T) {
	f := newFixture()

	r, err := f.usecase().Reject(ctx, mia, "c1", "  not during the sale  ")

	require.NoError(t, err)
	require.Equal(t, entities.StatusRejected, r.Status)
	require.Equal(t, "not during the sale", f.store.byID["c1"].Reason)
	require.Empty(t, f.executor.ran)
}

func Test_Reject_LosesTheRaceWhenAnotherApproverWonAfterTheRead(t *testing.T) {
	f := newFixture()
	stale := readingStale(f.store)
	_, _ = f.usecase().Approve(ctx, mia, "c1")

	_, lost := f.usecaseOver(stale).Reject(ctx, mia, "c1", "no")

	require.ErrorIs(t, lost, apperror.Conflict(""))
	require.Equal(t, entities.StatusExecuted, f.store.byID["c1"].Status)
}

func Test_Cancel_OnlyByThePersonWhoAsked(t *testing.T) {
	f := newFixture()

	_, notTheirs := f.usecase().Cancel(ctx, mia, "c1")
	r, err := f.usecase().Cancel(ctx, bob, "c1")

	require.ErrorIs(t, notTheirs, apperror.Forbidden(""))
	require.NoError(t, err)
	require.Equal(t, entities.StatusCancelled, r.Status)
	require.Equal(t, "cancelled by bob", r.Reason)
}

func Test_Get_HidesOtherTeamsFromATokenAndAwaitingListsWhatTheViewerDecides(t *testing.T) {
	f := newFixture()
	agent := actor.Actor{UserID: "u-bob", Token: actor.Token{ID: "k2", TeamIDs: []string{"t2"}}}

	_, hidden := f.usecase().Get(ctx, agent, "c1")
	forMia, _ := f.usecase().Awaiting(ctx, mia)
	forBob, _ := f.usecase().Awaiting(ctx, bob)

	require.ErrorIs(t, hidden, apperror.NotFound(""))
	require.Len(t, forMia, 1)
	require.Empty(t, forBob)
}

func Test_List_ATokenWithoutTeamsSeesNothing(t *testing.T) {
	f := newFixture()
	teamless := actor.Actor{UserID: "u-bob", Token: actor.Token{ID: "k3"}}

	listed, err := f.usecase().List(ctx, teamless, "")

	require.NoError(t, err)
	require.Empty(t, listed)
}

func Test_Sweep_ExpiresPendingAndFailsStuckApprovals(t *testing.T) {
	f := newFixture()
	decided := now.Add(-time.Hour)
	f.store.byID["c2"] = entities.Request{ID: "c2", Status: entities.StatusApproved, DecidedAt: &decided, ExpiresAt: now.Add(time.Hour)}
	clock.WithTime(now.Add(entities.Lifetime))

	swept, err := f.usecase().Sweep(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, swept)
	require.Equal(t, entities.StatusExpired, f.store.byID["c1"].Status)
	require.Equal(t, "platform", f.store.byID["c1"].DeciderLogin)
	require.Equal(t, entities.StatusFailed, f.store.byID["c2"].Status)
	require.ElementsMatch(t, []string{"request.expired", "request.failed"}, f.events.actions)
}

func Test_Sweep_LogsWhatItCouldNotSweep(t *testing.T) {
	f := newFixture()
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	f.store.broken = errors.New("db went away")
	clock.WithTime(now.Add(entities.Lifetime))

	swept, err := f.usecase().Sweep(ctx)

	require.NoError(t, err)
	require.Zero(t, swept)
	require.Contains(t, logged.String(), "c1")
	require.Contains(t, logged.String(), "db went away")
}

func Test_Sweep_IsQuietWhenSomeoneElseClosedTheRequest(t *testing.T) {
	f := newFixture()
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	f.store.broken = apperror.Conflict("this request was already decided")
	clock.WithTime(now.Add(entities.Lifetime))

	swept, err := f.usecase().Sweep(ctx)

	require.NoError(t, err)
	require.Zero(t, swept)
	require.Empty(t, logged.String())
}
