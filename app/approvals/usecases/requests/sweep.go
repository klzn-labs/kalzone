package requests

import (
	"context"
	"errors"
	"log"

	"github.com/adrianozp/platform/app/approvals/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
)

var platform = actor.Actor{Login: "platform"}

const interrupted = "the platform stopped while running this request, check the target before asking again"

func (u RequestsUsecase) Sweep(ctx context.Context) (int, error) {
	overdue, err := u.store.Overdue(ctx, clock.Now().UTC())
	if err != nil {
		return 0, err
	}
	swept := 0
	for _, r := range overdue {
		if u.swept(ctx, r) {
			swept++
		}
	}
	return swept, nil
}

func (u RequestsUsecase) swept(ctx context.Context, r entities.Request) bool {
	err := u.sweepOne(ctx, r)
	if err != nil && !errors.Is(err, apperror.Conflict("")) {
		log.Printf("change request %s: could not sweep it: %v", r.ID, err)
	}
	return err == nil
}

func (u RequestsUsecase) sweepOne(ctx context.Context, r entities.Request) error {
	if r.Pending() {
		_, err := u.close(ctx, platform, r, entities.StatusExpired, "nobody decided within 24 hours", "request.expired")
		return err
	}
	failed := r.Fail(string(apperror.CodeInternal), interrupted, clock.Now().UTC())
	if err := u.store.Transition(ctx, failed, entities.StatusApproved); err != nil {
		return err
	}
	u.publisher.Publish(ctx, entities.EventOf(platform, "request.failed", failed))
	return nil
}
