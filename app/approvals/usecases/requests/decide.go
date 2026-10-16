package requests

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/adrianozp/platform/app/approvals/entities"
	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/approval"
	"github.com/adrianozp/platform/pkg/clock"
)

func (u RequestsUsecase) Approve(ctx context.Context, a actor.Actor, id string) (entities.Request, error) {
	r, err := u.decidable(ctx, a, id)
	if err != nil {
		return entities.Request{}, err
	}
	approved := r.Approve(a, clock.Now().UTC())
	if err := u.store.Transition(ctx, approved, entities.StatusPending); err != nil {
		return entities.Request{}, err
	}
	u.publisher.Publish(ctx, entities.EventOf(a, "request.approved", approved))
	return u.execute(context.WithoutCancel(ctx), a, approved)
}

func (u RequestsUsecase) Reject(ctx context.Context, a actor.Actor, id, reason string) (entities.Request, error) {
	r, err := u.decidable(ctx, a, id)
	if err != nil {
		return entities.Request{}, err
	}
	return u.close(ctx, a, r, entities.StatusRejected, strings.TrimSpace(reason), "request.rejected")
}

func (u RequestsUsecase) Cancel(ctx context.Context, a actor.Actor, id string) (entities.Request, error) {
	r, err := u.Get(ctx, a, id)
	if err != nil {
		return entities.Request{}, err
	}
	if r.RequestedBy != a.UserID {
		return entities.Request{}, errNotYours
	}
	if !r.Pending() {
		return entities.Request{}, errNotPending
	}
	return u.close(ctx, a, r, entities.StatusCancelled, "cancelled by "+a.Login, "request.cancelled")
}

func (u RequestsUsecase) decidable(ctx context.Context, a actor.Actor, id string) (entities.Request, error) {
	r, err := u.Get(ctx, a, id)
	if err != nil {
		return entities.Request{}, err
	}
	if err := u.authz.Can(ctx, a, authz.ApproveRequest, authz.Resource{TeamID: r.TeamID}); err != nil {
		return entities.Request{}, err
	}
	if !r.Pending() {
		return entities.Request{}, errNotPending
	}
	if r.ExpiredAt(clock.Now().UTC()) {
		return entities.Request{}, errExpired
	}
	return r, nil
}

func (u RequestsUsecase) close(ctx context.Context, a actor.Actor, r entities.Request, status entities.Status, reason, action string) (entities.Request, error) {
	closed := r.Close(status, a, reason, clock.Now().UTC())
	if err := u.store.Transition(ctx, closed, entities.StatusPending); err != nil {
		return entities.Request{}, err
	}
	u.publisher.Publish(ctx, entities.EventOf(a, action, closed))
	return closed, nil
}

func (u RequestsUsecase) execute(ctx context.Context, approver actor.Actor, r entities.Request) (entities.Request, error) {
	result, err := u.run(ctx, approver, r)
	done, action := finished(r, result, err)
	if err := u.store.Transition(ctx, done, entities.StatusApproved); err != nil {
		return entities.Request{}, err
	}
	u.publisher.Publish(ctx, entities.EventOf(approver, action, done))
	return done, nil
}

func (u RequestsUsecase) run(ctx context.Context, approver actor.Actor, r entities.Request) (string, error) {
	executor, known := u.executors[r.Kind]
	if !known {
		return "", errNoExecutor
	}
	approved, err := u.approvedActor(ctx, approver, r)
	if err != nil {
		return "", err
	}
	return executor.Execute(ctx, approved, r.Payload)
}

func (u RequestsUsecase) approvedActor(ctx context.Context, approver actor.Actor, r entities.Request) (actor.Actor, error) {
	owner, err := u.tokens.ActorOf(ctx, r.TokenID)
	if err != nil {
		return actor.Actor{}, err
	}
	owner.Approval = actor.Approval{RequestID: r.ID, ApproverID: approver.UserID, ApproverLogin: approver.Login}
	return owner, nil
}

func finished(r entities.Request, result string, err error) (entities.Request, string) {
	now := clock.Now().UTC()
	if err != nil {
		return r.Fail(string(apperror.CodeOf(err)), messageOf(err), now), "request.failed"
	}
	return r.Succeed(result, now), "request.executed"
}

func messageOf(err error) string {
	var appErr *apperror.AppError
	if errors.As(err, &appErr) && appErr.Code != apperror.CodeInternal {
		return appErr.Message
	}
	log.Printf("change request failed: %v", err)
	return "internal error, see the control plane logs"
}

const serviceDeleted = "the service is being deleted"

func (u RequestsUsecase) CancelForService(ctx context.Context, a actor.Actor, serviceID string) error {
	pending, err := u.store.List(ctx, entities.Filter{Status: entities.StatusPending, ServiceID: serviceID, Limit: listLimit})
	if err != nil {
		return err
	}
	for _, r := range pending {
		if r.Kind == approval.KindRemove {
			continue
		}
		if _, err := u.close(ctx, a, r, entities.StatusCancelled, serviceDeleted, "request.cancelled"); err != nil {
			return err
		}
	}
	return nil
}
