package deployments

import (
	"context"

	"github.com/klzn-labs/kalzone/pkg/approval"
)

func (u DeploymentsUsecase) shipsFor(ctx context.Context, kind, fromReleaseID, toReleaseID string) *approval.Ships {
	if kind != approval.KindDeploy && kind != approval.KindPromote && kind != approval.KindRollback {
		return nil
	}
	ships, err := u.shipments.Ships(ctx, fromReleaseID, toReleaseID)
	if err != nil {
		return &approval.Ships{State: "failed", Direction: approval.ShipsForward, Releases: []approval.ShipRelease{}, Error: err.Error()}
	}
	return &ships
}
