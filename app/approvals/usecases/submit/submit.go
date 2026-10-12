package submit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/adrianozp/platform/app/approvals/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/approval"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/adrianozp/platform/pkg/events"
	"github.com/adrianozp/platform/pkg/uuid"
)

type Store interface {
	Create(ctx context.Context, r entities.Request) error
	PendingFor(ctx context.Context, serviceID, environment, subject string) ([]entities.Request, error)
	Transition(ctx context.Context, r entities.Request, from entities.Status) error
}

type Notifier interface {
	Notify(text string)
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

type SubmitUsecase struct {
	store     Store
	notifier  Notifier
	publisher Publisher
	baseURL   string
}

func NewSubmitUsecase(cfg config.Config, store Store, notifier Notifier, publisher Publisher) SubmitUsecase {
	return SubmitUsecase{store: store, notifier: notifier, publisher: publisher, baseURL: strings.TrimSuffix(cfg.BaseURL, "/")}
}

func (u SubmitUsecase) Submit(ctx context.Context, a actor.Actor, c approval.Change) (*approval.Pending, error) {
	r, err := newRequest(a, c)
	if err != nil {
		return nil, err
	}
	if err := u.store.Create(ctx, r); err != nil {
		return nil, err
	}
	u.supersede(ctx, a, r)
	u.publisher.Publish(ctx, entities.EventOf(a, "request.created", r))
	u.notifier.Notify(u.notice(r))
	return u.pending(r), nil
}

func newRequest(a actor.Actor, c approval.Change) (entities.Request, error) {
	payload, err := json.Marshal(c.Proposal)
	if err != nil {
		return entities.Request{}, err
	}
	now := clock.Now().UTC().Truncate(time.Millisecond)
	return entities.Request{
		ID: uuid.NewUUID(), Kind: c.Kind, Subject: c.Subject, ServiceID: c.ServiceID, ServiceSlug: c.ServiceSlug,
		TeamID: c.TeamID, Environment: c.Environment, Summary: entities.Clip(c.Summary, entities.SummaryLimit),
		Payload: payload, RequestedBy: a.UserID, RequesterLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name,
		Status: entities.StatusPending, CreatedAt: now, ExpiresAt: now.Add(entities.Lifetime),
	}, nil
}

func (u SubmitUsecase) supersede(ctx context.Context, a actor.Actor, newer entities.Request) {
	older, err := u.store.PendingFor(ctx, newer.ServiceID, newer.Environment, newer.Subject)
	if err != nil {
		log.Printf("change request %s: could not look for older requests to supersede: %v", newer.ID, err)
		return
	}
	for _, r := range older {
		if err := u.cancel(ctx, a, r, newer); err != nil {
			log.Printf("change request %s: could not supersede %s: %v", newer.ID, r.ID, err)
		}
	}
}

func (u SubmitUsecase) cancel(ctx context.Context, a actor.Actor, older, newer entities.Request) error {
	if older.ID == newer.ID || !olderThan(older, newer) {
		return nil
	}
	closed := older.Close(entities.StatusCancelled, a, "superseded by "+newer.ID, clock.Now().UTC())
	err := u.store.Transition(ctx, closed, entities.StatusPending)
	if errors.Is(err, apperror.Conflict("")) {
		return nil
	}
	if err != nil {
		return err
	}
	u.publisher.Publish(ctx, entities.EventOf(a, "request.superseded", closed))
	return nil
}

func olderThan(r, other entities.Request) bool {
	if r.CreatedAt.Equal(other.CreatedAt) {
		return r.ID < other.ID
	}
	return r.CreatedAt.Before(other.CreatedAt)
}

func (u SubmitUsecase) notice(r entities.Request) string {
	return fmt.Sprintf("%s via %s asks to %s on %s in %s. Approve or reject within 24 hours: %s",
		r.RequesterLogin, r.TokenName, r.Summary, r.ServiceSlug, r.Environment, u.baseURL+r.ApprovePath())
}

func (u SubmitUsecase) pending(r entities.Request) *approval.Pending {
	return &approval.Pending{
		Kind: "change_request", ID: r.ID, Status: string(r.Status), Summary: r.Summary,
		ApproveURL: u.baseURL + r.ApprovePath(), Poll: r.PollPath(), ExpiresAt: r.ExpiresAt,
	}
}
