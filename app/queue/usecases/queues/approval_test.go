package queues_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/stretchr/testify/require"
)

var approvedAna = actor.Actor{UserID: "u-ana", Login: "ana", Approval: actor.Approval{RequestID: "c1", ApproverLogin: "mia"}}

func (f fixture) proposal(t *testing.T) queues.Proposal {
	t.Helper()
	raw, err := json.Marshal(f.requests.changes[len(f.requests.changes)-1].Proposal)
	require.NoError(t, err)
	var p queues.Proposal
	require.NoError(t, json.Unmarshal(raw, &p))
	return p
}

func Test_Create_NeedingApprovalSubmitsTheDraft(t *testing.T) {
	f := newFixture(t, false)

	_, err := f.as(authz.NeedsApproval).Create(ctx, ana, draft())

	var pending *approval.Pending
	require.True(t, errors.As(err, &pending), "%v", err)
	require.Empty(t, f.store.queues)
	c := f.requests.changes[0]
	require.Equal(t, "queue.place-import", c.Subject)
	require.Equal(t, "create queue place-import", c.Summary)
	require.Equal(t, "worker", c.ServiceSlug)
	created, err := f.as(authz.Approved).ApplyApproved(ctx, approvedAna, f.proposal(t))
	require.NoError(t, err)
	require.Equal(t, 1, created.Version)
}

func Test_Update_NeedingApprovalKeepsTheBaseVersion(t *testing.T) {
	f := newFixture(t, false)
	created, _ := f.usecase.Create(ctx, ana, draft())
	settings := created.Settings
	settings.MaxAttempts = 9

	_, err := f.as(authz.NeedsApproval).Update(ctx, ana, "prod", "place-import", queues.Change{Settings: &settings, BaseVersion: 1})
	preview, _ := f.usecase.Preview(ctx, f.proposal(t))
	_, _ = f.usecase.Update(ctx, ana, "prod", "place-import", queues.Change{Settings: &created.Settings, BaseVersion: 1})
	_, stale := f.as(authz.Approved).ApplyApproved(ctx, approvedAna, f.proposal(t))

	require.Error(t, err)
	require.Equal(t, []approval.Line{{Key: "max_attempts", Before: "5", After: "9"}}, preview.Lines)
	require.ErrorIs(t, stale, apperror.Conflict(""))
}

func Test_Delete_NeedingApprovalStillRefusesAQueueWithMessages(t *testing.T) {
	f := newFixture(t, false)
	created, _ := f.usecase.Create(ctx, ana, draft())
	f.stats[created.ID] = fresh(3)

	err := f.as(authz.NeedsApproval).Delete(ctx, ana, "prod", "place-import", false)

	require.ErrorIs(t, err, apperror.Conflict(""))
	require.Empty(t, f.requests.changes)
}

func Test_PauseAndDeadLettersStayRefusedWithoutBypass(t *testing.T) {
	f := newFixture(t, false)
	_, _ = f.usecase.Create(ctx, ana, draft())

	_, paused := f.as(authz.NeedsApproval).SetPaused(ctx, ana, "prod", "place-import", true)
	purged := f.as(authz.NeedsApproval).PurgeDead(ctx, ana, "prod", "place-import")

	require.Equal(t, apperror.CodeApprovalRequired, apperror.CodeOf(paused))
	require.Equal(t, apperror.CodeApprovalRequired, apperror.CodeOf(purged))
	require.Empty(t, f.requests.changes)
}

func Test_QueueEdits_PublishTheBypassWhenMade(t *testing.T) {
	f := newFixture(t, false)

	_, err := f.as(authz.Bypassed).Create(ctx, ana, draft())

	require.NoError(t, err)
	require.Equal(t, []string{"queue.created", "token.bypass_used"}, f.events.actions)
}

func Test_ApplyApproved_RefusesWhenTheGateIsNotApprovedAndNeverProposes(t *testing.T) {
	f := newFixture(t, false)
	_, _ = f.as(authz.NeedsApproval).Create(ctx, ana, draft())
	proposal := f.proposal(t)
	before := len(f.requests.changes)

	_, err := f.as(authz.NeedsApproval).ApplyApproved(ctx, approvedAna, proposal)

	require.ErrorIs(t, err, apperror.Forbidden(""))
	require.Len(t, f.requests.changes, before)
	require.Empty(t, f.store.queues)
}

func Test_Create_NeedingApprovalPreviewListsThePublishers(t *testing.T) {
	f := newFixture(t, false)
	_, _ = f.as(authz.NeedsApproval).Create(ctx, ana, draft())

	preview, err := f.usecase.Preview(ctx, f.proposal(t))

	require.NoError(t, err)
	require.Contains(t, preview.Lines, approval.Line{Key: "publishers", After: "worker, importer"})
}

func Test_Update_NeedingApprovalPreviewShowsPublishersNowAndAfter(t *testing.T) {
	f := newFixture(t, false)
	_, _ = f.usecase.Create(ctx, ana, draft())
	none := []string{}

	_, _ = f.as(authz.NeedsApproval).Update(ctx, ana, "prod", "place-import", queues.Change{Publishers: &none, BaseVersion: 1})
	preview, err := f.usecase.Preview(ctx, f.proposal(t))

	require.NoError(t, err)
	require.Equal(t, []approval.Line{{Key: "publishers", Before: "worker, importer", After: "worker"}}, preview.Lines)
}

func Test_Delete_NeedingApprovalFlagsForce(t *testing.T) {
	f := newFixture(t, false)
	_, _ = f.usecase.Create(ctx, ana, draft())

	_ = f.as(authz.NeedsApproval).Delete(ctx, ana, "prod", "place-import", true)
	preview, err := f.usecase.Preview(ctx, f.proposal(t))

	require.NoError(t, err)
	require.Equal(t, "delete queue place-import (force)", f.requests.changes[0].Summary)
	require.Contains(t, preview.Lines, approval.Line{Key: "force", After: "queued messages will be dropped"})
}
