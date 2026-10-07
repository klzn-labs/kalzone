package ownership

import "context"

type ServiceCounter interface {
	CountOwnedBy(ctx context.Context, teamID string) (int64, error)
}

type OwnershipUsecase struct {
	services ServiceCounter
}

func NewOwnershipUsecase(services ServiceCounter) OwnershipUsecase {
	return OwnershipUsecase{services: services}
}

func (u OwnershipUsecase) CountOwnedBy(ctx context.Context, teamID string) (int64, error) {
	return u.services.CountOwnedBy(ctx, teamID)
}
