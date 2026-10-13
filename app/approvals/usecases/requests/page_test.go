package requests_test

import (
	"testing"

	"github.com/adrianozp/platform/app/approvals/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/page"
	"github.com/stretchr/testify/require"
)

func Test_PageFor_FiltersByAKnownStatus(t *testing.T) {
	f := newFixture()
	done := pendingRequest("c2", "k1")
	done.Status = entities.StatusExecuted
	f.store.byID["c2"] = done

	pending, err := f.usecase().PageFor(ctx, mia, "pending", page.First(10))
	require.NoError(t, err)
	_, bogus := f.usecase().PageFor(ctx, mia, "bogus", page.First(10))
	empty, err := f.usecase().PageFor(ctx, actor.Actor{UserID: "u-bob", Token: actor.Token{ID: "k9"}}, "", page.First(10))
	require.NoError(t, err)

	require.Len(t, pending.Items, 1)
	require.Equal(t, "c1", pending.Items[0].ID)
	require.Equal(t, "status", bogus.(*apperror.AppError).Field)
	require.Equal(t, "send one of pending, approved, executed, failed, rejected, cancelled, expired, or leave status out", bogus.(*apperror.AppError).Hint)
	require.ErrorIs(t, bogus, apperror.Invalid(""))
	require.NotNil(t, empty.Items)
	require.Empty(t, empty.Items)
}
