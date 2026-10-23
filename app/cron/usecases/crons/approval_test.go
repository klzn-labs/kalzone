package crons_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/cron/usecases/crons"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/stretchr/testify/require"
)

var approvedAna = actor.Actor{UserID: "u-ana", Teams: []string{"t-plateful"}, Login: "ana", Approval: actor.Approval{RequestID: "c1", ApproverLogin: "mia"}}

func (f fixture) proposal(t *testing.T) crons.Proposal {
	t.Helper()
	raw, err := json.Marshal(f.requests.changes[len(f.requests.changes)-1].Proposal)
	require.NoError(t, err)
	var p crons.Proposal
	require.NoError(t, json.Unmarshal(raw, &p))
	return p
}

func requirePending(t *testing.T, err error) {
	t.Helper()
	var pending *approval.Pending
	require.True(t, errors.As(err, &pending), "%v", err)
}

func Test_Create_NeedingApprovalSubmitsTheDraft(t *testing.T) {
	f := newFixture(t)
	d := draft()
	d.Payload = []byte(`{"full":true}`)

	_, err := f.as(authz.NeedsApproval).Create(ctx, ana, d)
	preview, _ := f.usecase.Preview(ctx, f.proposal(t))
	created, applyErr := f.as(authz.Approved).ApplyApproved(ctx, approvedAna, f.proposal(t))

	requirePending(t, err)
	c := f.requests.changes[0]
	require.Equal(t, approval.KindCronCreate, c.Kind)
	require.Equal(t, "cron.nightly-import", c.Subject)
	require.Equal(t, "create cron nightly-import", c.Summary)
	require.Equal(t, []string{"s-worker", "worker"}, []string{c.ServiceID, c.ServiceSlug})
	require.Contains(t, preview.Lines, approval.Line{Key: "timeout", After: "1m0s"})
	require.Contains(t, preview.Lines, approval.Line{Key: "payload", After: `{"full":true}`})
	require.NoError(t, applyErr)
	require.Equal(t, `{"full":true}`, string(created.Payload))
	require.Equal(t, "u-ana", f.events.events[0].ActorID)
	require.Equal(t, "mia", f.events.events[0].Details["approved_by"])
}

func Test_Update_NeedingApprovalPreviewsTheChangedFields(t *testing.T) {
	f := newFixture(t)
	_, _ = f.usecase.Create(ctx, ana, draft())

	_, err := f.as(authz.NeedsApproval).Update(ctx, ana, "prod", "nightly-import", crons.Change{Schedule: ptr("@daily"), Timeout: &minute})
	preview, _ := f.usecase.Preview(ctx, f.proposal(t))
	updated, applyErr := f.as(authz.Approved).ApplyApproved(ctx, approvedAna, f.proposal(t))

	requirePending(t, err)
	require.Equal(t, "change cron nightly-import", f.requests.changes[0].Summary)
	require.Equal(t, []approval.Line{{Key: "schedule", Before: "0 3 * * *", After: "@daily"}}, preview.Lines)
	require.NoError(t, applyErr)
	require.Equal(t, "@daily", updated.Schedule)
}

func Test_Update_PreviewWarnsWhenTheChangeNoLongerFits(t *testing.T) {
	f := newFixture(t)
	_, _ = f.usecase.Create(ctx, ana, draft())
	_, _ = f.as(authz.NeedsApproval).Update(ctx, ana, "prod", "nightly-import", crons.Change{Timeout: ptr(10 * time.Minute)})
	proposal := f.proposal(t)
	proposal.Change.Timeout = ptr(time.Hour)

	preview, err := f.usecase.Preview(ctx, proposal)

	require.NoError(t, err)
	require.Contains(t, preview.Conflict, "approving will fail")
}

func Test_Delete_NeedingApprovalKeepsTheCronUntilApproved(t *testing.T) {
	f := newFixture(t)
	_, _ = f.usecase.Create(ctx, ana, draft())

	err := f.as(authz.NeedsApproval).Delete(ctx, ana, "prod", "nightly-import")
	kept := len(f.store.crons)
	preview, _ := f.usecase.Preview(ctx, f.proposal(t))
	_, applyErr := f.as(authz.Approved).ApplyApproved(ctx, approvedAna, f.proposal(t))

	requirePending(t, err)
	require.Equal(t, 1, kept)
	require.Equal(t, approval.Line{Key: "cron", Before: "nightly-import", After: "deleted"}, preview.Lines[0])
	require.NoError(t, applyErr)
	require.Empty(t, f.store.crons)
}

func Test_ApplyApproved_RefusesAnUnapprovedActor(t *testing.T) {
	f := newFixture(t)
	_, _ = f.as(authz.NeedsApproval).Create(ctx, ana, draft())

	_, err := f.as(authz.NeedsApproval).ApplyApproved(ctx, ana, f.proposal(t))

	require.ErrorIs(t, err, apperror.Forbidden(""))
	require.Empty(t, f.store.crons)
}

func Test_ApplyApproved_RefusesAnUnknownKind(t *testing.T) {
	f := newFixture(t)
	_, _ = f.usecase.Create(ctx, ana, draft())

	_, err := f.usecase.ApplyApproved(ctx, ana, crons.Proposal{Kind: "cron.explode", Environment: "prod", Name: "nightly-import"})
	_, previewErr := f.usecase.Preview(ctx, crons.Proposal{Kind: "cron.explode"})

	require.ErrorIs(t, err, apperror.Invalid(""))
	require.ErrorIs(t, previewErr, apperror.Invalid(""))
}
