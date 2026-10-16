package requests_test

import (
	"testing"

	"github.com/adrianozp/platform/app/approvals/entities"
	"github.com/adrianozp/platform/pkg/approval"
	"github.com/stretchr/testify/require"
)

func Test_CancelForService_ClosesItsPendingRequestsButNotItsRemovals(t *testing.T) {
	f := newFixture()
	deploy := pendingRequest("c2", "k1")
	deploy.ServiceID = "s1"
	removal := pendingRequest("c3", "k1")
	removal.ServiceID, removal.Kind = "s1", approval.KindRemove
	other := pendingRequest("c4", "k1")
	other.ServiceID = "s2"
	for _, r := range []entities.Request{deploy, removal, other} {
		f.store.byID[r.ID] = r
	}

	require.NoError(t, f.usecase().CancelForService(ctx, mia, "s1"))

	require.Equal(t, entities.StatusCancelled, f.store.byID["c2"].Status)
	require.Equal(t, "the service is being deleted", f.store.byID["c2"].Reason)
	require.Equal(t, entities.StatusPending, f.store.byID["c3"].Status)
	require.Equal(t, entities.StatusPending, f.store.byID["c4"].Status)
}
