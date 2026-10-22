package databases_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/stretchr/testify/require"
)

var approved = actor.Actor{UserID: "u-ana", Teams: []string{"t-plateful", "t-reviews"}, Login: "ana", Approval: actor.Approval{RequestID: "c1", ApproverLogin: "bob"}}

func prodDraft() databases.Draft {
	d := shopDraft()
	d.Environment = "prod"
	return d
}

func Test_ApplyApproved_CreatesWithAFreshPassword(t *testing.T) {
	f := newFixture(t)
	d := prodDraft()
	o := databases.Proposal{Kind: approval.KindDatabaseCreate, Environment: "prod", Name: "shop", Draft: &d}

	db, err := f.as(authz.Approved).ApplyApproved(ctx, approved, o)

	require.NoError(t, err)
	require.Equal(t, "shop", db.Schema)
	require.Len(t, f.provisioner.created[0].password, 64)
	require.Equal(t, "c1", f.events.events[0].Details["request_id"])
}

func Test_ApplyApproved_RefusesWithoutApproval(t *testing.T) {
	f := newFixture(t)
	d := prodDraft()
	o := databases.Proposal{Kind: approval.KindDatabaseCreate, Environment: "prod", Name: "shop", Draft: &d}

	_, err := f.as(authz.NeedsApproval).ApplyApproved(ctx, ana, o)

	require.Equal(t, apperror.CodeForbidden, apperror.CodeOf(err))
	require.Zero(t, f.provisioner.writes)
}

func Test_ApplyApproved_ReplaysAttachDetachAndDelete(t *testing.T) {
	f := newFixture(t)
	f.created(t, prodDraft())
	u := f.as(authz.Approved)
	proposal := func(kind string) databases.Proposal {
		return databases.Proposal{Kind: kind, Environment: "prod", Name: "shop", Service: "shop"}
	}

	_, err := u.ApplyApproved(ctx, approved, proposal(approval.KindDatabaseAttach))
	require.NoError(t, err)
	require.True(t, f.store.attachments[0].Active())
	_, err = u.ApplyApproved(ctx, approved, proposal(approval.KindDatabaseDetach))
	require.NoError(t, err)
	require.False(t, f.store.attachments[0].Active())
	_, err = u.ApplyApproved(ctx, approved, proposal(approval.KindDatabaseDelete))
	require.NoError(t, err)
	require.Empty(t, f.store.databases)
}

func Test_ApplyApproved_RefusesUnknownKinds(t *testing.T) {
	f := newFixture(t)
	f.created(t, prodDraft())

	_, err := f.as(authz.Approved).ApplyApproved(ctx, approved, databases.Proposal{Kind: "queue.create", Environment: "prod", Name: "shop"})

	require.Equal(t, apperror.CodeInvalidInput, apperror.CodeOf(err))
}

func Test_Preview_Create(t *testing.T) {
	f := newFixture(t)
	d := prodDraft()

	preview, err := f.open().Preview(ctx, databases.Proposal{Kind: approval.KindDatabaseCreate, Environment: "prod", Name: "shop", Draft: &d})

	require.NoError(t, err)
	require.Equal(t, []approval.Line{
		{Key: "database", After: "shop"}, {Key: "schema", After: "shop"},
		{Key: "team", After: "plateful"}, {Key: "max_connections", After: "10"},
	}, preview.Lines)
}

func Test_Preview_AttachAndDelete(t *testing.T) {
	f := newFixture(t)
	f.created(t, prodDraft())

	attach, err := f.open().Preview(ctx, databases.Proposal{Kind: approval.KindDatabaseAttach, Environment: "prod", Name: "shop", Service: "worker"})
	require.NoError(t, err)
	remove, err := f.open().Preview(ctx, databases.Proposal{Kind: approval.KindDatabaseDelete, Environment: "prod", Name: "shop"})
	require.NoError(t, err)

	require.Contains(t, attach.Lines, approval.Line{Key: "service", After: "worker"})
	require.Contains(t, remove.Lines, approval.Line{Key: "schema", Before: "shop", After: "dropped"})
}

func Test_ApplyApproved_ReleasesAnAdoptedDatabase(t *testing.T) {
	f := adoptFixture(t)
	d := platefulDraft()
	d.Services = nil
	f.adopt(t, d)
	o := databases.Proposal{Kind: approval.KindDatabaseDelete, Environment: "prod", Name: "plateful"}

	_, err := f.as(authz.Approved).ApplyApproved(ctx, approved, o)

	require.NoError(t, err)
	require.Equal(t, []string{"kz_plateful"}, f.provisioner.released)
	require.Empty(t, f.provisioner.dropped)
	require.Empty(t, f.store.databases)
	require.Equal(t, "c1", f.events.events[1].Details["request_id"])
}

func Test_Preview_ReleaseKeepsTheSchema(t *testing.T) {
	f := adoptFixture(t)
	f.adopt(t, platefulDraft())

	preview, err := f.open().Preview(ctx, databases.Proposal{Kind: approval.KindDatabaseDelete, Environment: "prod", Name: "plateful"})

	require.NoError(t, err)
	require.Contains(t, preview.Lines, approval.Line{Key: "database", Before: "plateful", After: "released"})
	require.Contains(t, preview.Lines, approval.Line{Key: "schema", Before: "plateful", After: "kept"})
}
