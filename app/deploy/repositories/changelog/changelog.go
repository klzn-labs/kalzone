package changelog

import (
	"context"

	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Between interface {
	Between(ctx context.Context, fromReleaseID, toReleaseID string) (approval.Ships, error)
}

type Shipments struct {
	changelogs Between
}

func NewShipments(changelogs Between) Shipments {
	return Shipments{changelogs: changelogs}
}

func (s Shipments) Ships(ctx context.Context, fromReleaseID, toReleaseID string) (approval.Ships, error) {
	return s.changelogs.Between(ctx, fromReleaseID, toReleaseID)
}
