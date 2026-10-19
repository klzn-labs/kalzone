package queues_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func attachedFixture(t *testing.T, publishers ...string) (fixture, string) {
	t.Helper()
	f := newFixture(t, false)
	d := draft()
	d.Publishers = publishers
	q, err := f.usecase.Create(ctx, ana, d)
	require.NoError(t, err)
	return f, q.ID
}

func Test_Delete_RefusesWhilePublishersAreAttached(t *testing.T) {
	f, _ := attachedFixture(t, "importer")

	err := f.usecase.Delete(ctx, ana, "prod", "place-import", false)

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
	var appErr *apperror.AppError
	require.ErrorAs(t, err, &appErr)
	require.Contains(t, appErr.Hint, "importer")
	require.NotContains(t, appErr.Hint, "worker")
	require.Len(t, f.store.commands, 0)
	require.Len(t, f.store.queues, 1)
}

func Test_Delete_ConsumerAloneDoesNotBlock(t *testing.T) {
	f, id := attachedFixture(t)
	f.stats[id] = fresh(0)

	require.NoError(t, f.usecase.Delete(ctx, ana, "prod", "place-import", false))
}

func Test_Delete_ForceDoesNotBypassAttached(t *testing.T) {
	f, _ := attachedFixture(t, "importer")

	err := f.usecase.Delete(ctx, ana, "prod", "place-import", true)

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
}

func Test_Delete_AttachedCheckedBeforeApproval(t *testing.T) {
	f, id := attachedFixture(t, "importer")
	f.stats[id] = fresh(0)

	err := f.as(authz.NeedsApproval).Delete(ctx, ana, "prod", "place-import", false)

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(err))
	require.Empty(t, f.requests.changes)
}

func Test_Delete_AllowedOnceThePublisherIsDropped(t *testing.T) {
	f, id := attachedFixture(t, "importer")
	f.stats[id] = fresh(0)
	require.NoError(t, f.usecase.Detach(ctx, ana, services["importer"].ID))

	require.NoError(t, f.usecase.Delete(ctx, ana, "prod", "place-import", false))
}
