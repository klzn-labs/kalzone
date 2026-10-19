package deployments_test

import (
	"context"
	"errors"
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/deployments"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/stretchr/testify/require"
)

type shipments struct {
	calls [][2]string
	fail  bool
}

func (s *shipments) Ships(_ context.Context, fromReleaseID, toReleaseID string) (approval.Ships, error) {
	s.calls = append(s.calls, [2]string{fromReleaseID, toReleaseID})
	if s.fail {
		return approval.Ships{}, errors.New("release store down")
	}
	return approval.Ships{State: "ahead", Direction: approval.ShipsForward, To: 2, CommitCount: 3}, nil
}

func Test_Propose_FreezesShipsInThePayload(t *testing.T) {
	g := newGated()
	g.live(t, "prod", "r1")

	_, err := g.as(authz.NeedsApproval).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "prod", ReleaseID: "r2"})

	require.ErrorAs(t, err, new(*approval.Pending))
	require.Equal(t, [][2]string{{"r1", "r2"}}, g.shipments.calls)
	require.Equal(t, &approval.Ships{State: "ahead", Direction: "forward", To: 2, CommitCount: 3}, g.proposal(t).Ships)
}

func Test_Propose_RemoveHasNoShips(t *testing.T) {
	g := newGated()
	g.live(t, "prod", "r1")

	_, err := g.as(authz.NeedsApproval).Remove(ctx, ana, "api", "prod")

	require.ErrorAs(t, err, new(*approval.Pending))
	require.Empty(t, g.shipments.calls)
	require.Nil(t, g.proposal(t).Ships)
}

func Test_Propose_ShipmentErrorStillFilesTheRequest(t *testing.T) {
	g := newGated()
	g.live(t, "prod", "r1")
	g.shipments.fail = true

	_, err := g.as(authz.NeedsApproval).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "prod", ReleaseID: "r2"})

	require.ErrorAs(t, err, new(*approval.Pending))
	ships := g.proposal(t).Ships
	require.Equal(t, "failed", ships.State)
	require.Equal(t, "release store down", ships.Error)
}

func Test_Preview_ReturnsFrozenShips(t *testing.T) {
	g := newGated()
	g.live(t, "prod", "r1")
	_, _ = g.as(authz.NeedsApproval).Deploy(ctx, ana, deployments.Request{Service: "api", Environment: "prod", ReleaseID: "r2"})

	preview, err := g.as(authz.Open).Preview(ctx, g.proposal(t))

	require.NoError(t, err)
	require.Equal(t, 3, preview.Ships.CommitCount)
}

func Test_Preview_KeepsFrozenShipsWhenTheLiveStateIsGone(t *testing.T) {
	g := newGated()
	frozen := &approval.Ships{State: "first", To: 1}

	preview, err := g.as(authz.Open).Preview(ctx, deployments.Proposal{Kind: approval.KindDeploy, Service: "gone", Environment: "prod", Ships: frozen})

	require.Error(t, err)
	require.Equal(t, frozen, preview.Ships)
}
