package databases_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/stretchr/testify/require"
)

func Test_Delete_DropsTheSchemaAndTheRow(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())

	require.NoError(t, f.open().Delete(ctx, ana, "dev", "shop"))

	require.Equal(t, []string{"shop_dev/kz_shop_dev"}, f.provisioner.dropped)
	require.Empty(t, f.store.databases)
	require.Equal(t, []string{"database.created", "database.deleted"}, f.events.actions())
}

func Test_Delete_ForbiddenChangesNothing(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())
	f.attached(t, "dev", "shop", "shop")
	_, _ = f.open().Detach(ctx, ana, "dev", "shop", "shop")

	err := f.with(policy{denied: map[authz.Action]string{authz.EditDatabase: ""}}).Delete(ctx, ana, "dev", "shop")

	require.Equal(t, apperror.CodeForbidden, apperror.CodeOf(err))
	require.Empty(t, f.store.forgotten)
	require.Empty(t, f.provisioner.dropped)
	require.Len(t, f.store.databases, 1)
}

func Test_Delete_RefusesWhileAttached(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())
	f.attached(t, "dev", "shop", "shop", "worker")

	err := f.open().Delete(ctx, ana, "dev", "shop")

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
	require.Equal(t, "the database still has attached services", err.Error())
	require.Equal(t, "detach these first: shop, worker", hintOf(t, err))
	require.Empty(t, f.provisioner.dropped)
	require.Len(t, f.store.databases, 1)
}

func Test_Delete_RefusesWhileDetachedServiceStillRuns(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())
	f.attached(t, "dev", "shop", "shop", "worker")
	_, _ = f.open().Detach(ctx, ana, "dev", "shop", "shop")
	_, _ = f.open().Detach(ctx, ana, "dev", "shop", "worker")
	f.running["s-worker"] = true

	err := f.open().Delete(ctx, ana, "dev", "shop")

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
	require.Equal(t, "services still run with this database's credentials", err.Error())
	require.Equal(t, "redeploy or remove these first: worker", hintOf(t, err))
	require.Empty(t, f.store.forgotten)
	require.Empty(t, f.provisioner.dropped)
}

func Test_Delete_ForgetsDrainedAttachments(t *testing.T) {
	f := newFixture(t)
	f.created(t, shopDraft())
	f.attached(t, "dev", "shop", "shop")
	_, _ = f.open().Detach(ctx, ana, "dev", "shop", "shop")

	require.NoError(t, f.open().Delete(ctx, ana, "dev", "shop"))

	require.Equal(t, []string{"s-shop"}, f.store.forgotten)
	require.Len(t, f.provisioner.dropped, 1)
}

func Test_Delete_AttachedCheckedBeforeApproval(t *testing.T) {
	f := newFixture(t)
	d := shopDraft()
	d.Environment = "prod"
	f.created(t, d)
	f.attached(t, "prod", "shop", "shop")

	err := f.as(authz.NeedsApproval).Delete(ctx, ana, "prod", "shop")

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
	require.Empty(t, f.requests.changes)
}

func Test_Delete_ProtectedEnvSubmits(t *testing.T) {
	f := newFixture(t)
	d := shopDraft()
	d.Environment = "prod"
	f.created(t, d)

	err := f.as(authz.NeedsApproval).Delete(ctx, ana, "prod", "shop")

	var pending *approval.Pending
	require.ErrorAs(t, err, &pending)
	require.Equal(t, approval.KindDatabaseDelete, f.requests.changes[0].Kind)
	require.Empty(t, f.provisioner.dropped)
	require.Len(t, f.store.databases, 1)
}

func Test_Delete_RefusesAnAdoptedDatabase(t *testing.T) {
	f := newFixture(t)
	db := f.created(t, shopDraft())
	db.Adopted = true
	f.store.databases[db.ID] = db

	err := f.open().Delete(ctx, ana, "dev", "shop")

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
	require.Empty(t, f.provisioner.dropped)
	require.Len(t, f.store.databases, 1)
}
