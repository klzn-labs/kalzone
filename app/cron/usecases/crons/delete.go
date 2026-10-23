package crons

import (
	"context"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

func (u CronsUsecase) Delete(ctx context.Context, a actor.Actor, environment, name string) error {
	c, gate, err := u.editable(ctx, a, environment, name)
	if err != nil {
		return err
	}
	if gate == authz.NeedsApproval {
		return u.propose(ctx, a, c, Proposal{Kind: approval.KindCronDelete})
	}
	return u.remove(ctx, a, gate, c, nil)
}

func (u CronsUsecase) DeleteForService(ctx context.Context, a actor.Actor, serviceID string) error {
	list, err := u.store.ForService(ctx, serviceID)
	if err != nil {
		return err
	}
	for _, c := range list {
		if err := u.remove(ctx, a, authz.Open, c, map[string]any{"reason": "service deleted"}); err != nil {
			return err
		}
	}
	return nil
}

func (u CronsUsecase) remove(ctx context.Context, a actor.Actor, gate authz.Gate, c entities.Cron, details map[string]any) error {
	if err := u.store.Delete(ctx, c.ID); err != nil {
		return err
	}
	u.publish(ctx, a, gate, "cron.deleted", c, details)
	return nil
}
