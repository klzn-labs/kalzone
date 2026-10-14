package requests_test

import (
	"testing"

	"github.com/adrianozp/platform/app/approvals/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_RequestState_SaysWhetherTheRequestCanStillChange(t *testing.T) {
	f := newFixture()
	done := pendingRequest("c2", "k1")
	done.Status = entities.StatusRejected
	f.store.byID["c2"] = done
	outsider := actor.Actor{UserID: "u-zed", Token: actor.Token{ID: "k9", TeamIDs: []string{"t9"}}}

	status, final, err := f.usecase().RequestState(ctx, mia, "c1")
	require.NoError(t, err)
	rejected, closed, err := f.usecase().RequestState(ctx, mia, "c2")
	require.NoError(t, err)
	_, _, hidden := f.usecase().RequestState(ctx, outsider, "c1")

	require.Equal(t, "pending", status)
	require.False(t, final)
	require.Equal(t, "rejected", rejected)
	require.True(t, closed)
	require.ErrorIs(t, hidden, apperror.NotFound(""))
}
