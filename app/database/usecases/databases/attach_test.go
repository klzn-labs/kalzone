package databases_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/stretchr/testify/require"
)

func Test_Attach_RecordsAnActiveAttachment(t *testing.T) {
	f := newFixture(t)
	db := f.created(t, shopDraft())

	f.attached(t, "dev", "shop", "shop")

	require.Len(t, f.store.attachments, 1)
	require.Equal(t, db.ID, f.store.attachments[0].DatabaseID)
	require.Equal(t, "s-shop", f.store.attachments[0].ServiceID)
	require.True(t, f.store.attachments[0].Active())
	last := f.events.events[len(f.events.events)-1]
	require.Equal(t, "database.attached", last.Action)
	require.Equal(t, map[string]any{"service": "shop", "takes_effect": "next deploy"}, last.Details)
}

func Test_Attach_NeedsRightsOnTheService(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())
	denied := policy{denied: map[authz.Action]string{authz.EditSecrets: "t-reviews"}}

	_, err := f.with(denied).Attach(ctx, ana, "dev", "shop", "other")

	require.Equal(t, apperror.CodeForbidden, apperror.CodeOf(err))
	require.Empty(t, f.store.attachments)
	_, err = f.with(denied).Attach(ctx, ana, "dev", "shop", "worker")
	require.NoError(t, err)
}

func Test_Attach_NeedsRightsOnTheDatabase(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())

	_, err := f.with(policy{denied: map[authz.Action]string{authz.EditDatabase: ""}}).Attach(ctx, ana, "dev", "shop", "shop")

	require.Equal(t, apperror.CodeForbidden, apperror.CodeOf(err))
	require.Empty(t, f.store.attachments)
}

func Test_Attach_OneDatabasePerServiceAndEnv(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())
	f.created(t, databases.Draft{Name: "billing", Environment: "dev", Team: "plateful"})
	f.attached(t, "dev", "shop", "shop")

	_, err := f.open().Attach(ctx, ana, "dev", "billing", "shop")

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
	require.Equal(t, "detach shop first", hintOf(t, err))
	require.Len(t, f.store.attachments, 1)
}

func Test_Attach_SameDatabaseAgainIsANoop(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())
	f.attached(t, "dev", "shop", "shop")

	_, err := f.open().Attach(ctx, ana, "dev", "shop", "shop")

	require.NoError(t, err)
	require.Equal(t, []string{"database.created", "database.attached"}, f.events.actions())
}

func Test_Attach_ProtectedEnvSubmits(t *testing.T) {
	f := newFixture(t)
	d := shopDraft()
	d.Environment = "prod"
	f.created(t, d)

	_, err := f.as(authz.NeedsApproval).Attach(ctx, ana, "prod", "shop", "shop")

	var pending *approval.Pending
	require.ErrorAs(t, err, &pending)
	require.Empty(t, f.store.attachments)
	change := f.requests.changes[0]
	require.Equal(t, approval.KindDatabaseAttach, change.Kind)
	require.Equal(t, "database shop", change.Resource)
	require.Equal(t, databases.Proposal{Kind: approval.KindDatabaseAttach, Environment: "prod", Name: "shop", Service: "shop"}, change.Proposal)
}

func Test_Detach_StampsTheAttachment(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())
	f.attached(t, "dev", "shop", "shop")

	_, err := f.open().Detach(ctx, ana, "dev", "shop", "shop")

	require.NoError(t, err)
	require.False(t, f.store.attachments[0].Active())
	require.Equal(t, "database.detached", f.events.events[len(f.events.events)-1].Action)
}

func Test_Detach_NotAttachedIsNotFound(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())

	_, err := f.open().Detach(ctx, ana, "dev", "shop", "shop")

	require.Equal(t, apperror.CodeNotFound, apperror.CodeOf(err))
	require.Empty(t, hintOf(t, err))
	require.Equal(t, []string{"database.created"}, f.events.actions())
}

func Test_Detach_OtherDatabaseIsNotFoundNamingIt(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())
	f.created(t, databases.Draft{Name: "billing", Environment: "dev", Team: "plateful"})
	f.attached(t, "dev", "billing", "shop")

	_, err := f.open().Detach(ctx, ana, "dev", "shop", "shop")

	require.Equal(t, apperror.CodeNotFound, apperror.CodeOf(err))
	require.Equal(t, "shop uses database billing", hintOf(t, err))
	require.True(t, f.store.attachments[0].Active())
}

func Test_Detach_ProtectedEnvSubmits(t *testing.T) {
	f := newFixture(t)
	d := shopDraft()
	d.Environment = "prod"
	f.created(t, d)
	f.attached(t, "prod", "shop", "shop")

	_, err := f.as(authz.NeedsApproval).Detach(ctx, ana, "prod", "shop", "shop")

	var pending *approval.Pending
	require.ErrorAs(t, err, &pending)
	require.True(t, f.store.attachments[0].Active())
	require.Equal(t, approval.KindDatabaseDetach, f.requests.changes[0].Kind)
}

func Test_DetachService_DetachesEveryEnvironment(t *testing.T) {
	f := newFixture(t)
	prod := shopDraft()
	prod.Environment = "prod"
	f.created(t, shopDraft())
	f.created(t, prod)
	f.attached(t, "dev", "shop", "shop", "worker")
	f.attached(t, "prod", "shop", "shop")

	require.NoError(t, f.open().DetachService(ctx, ana, "s-shop"))

	for _, a := range f.store.attachments {
		require.Equal(t, a.ServiceID == "s-worker", a.Active(), a.ServiceID)
	}
	require.Len(t, f.events.events, 7)
	require.Equal(t, "database.detached", f.events.events[6].Action)
}
