package crons

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

type Draft struct {
	Name        string        `json:"name"`
	Environment string        `json:"environment"`
	Service     string        `json:"service"`
	Schedule    string        `json:"schedule"`
	Path        string        `json:"path"`
	Payload     []byte        `json:"payload,omitempty"`
	Timeout     time.Duration `json:"timeout,omitempty"`
}

type Change struct {
	Schedule *string        `json:"schedule,omitempty"`
	Path     *string        `json:"path,omitempty"`
	Payload  *[]byte        `json:"payload,omitempty"`
	Timeout  *time.Duration `json:"timeout,omitempty"`
}

var (
	errNameTaken      = apperror.Conflict("a cron with this name already exists in this environment")
	errDecommissioned = apperror.Conflict("the service is being deleted").WithHint("choose another service")
)

func (u CronsUsecase) Create(ctx context.Context, a actor.Actor, d Draft) (entities.Cron, error) {
	target, err := u.target(ctx, d)
	if err != nil {
		return entities.Cron{}, err
	}
	gate, err := u.authz.Gate(ctx, a, authz.EditCron, authz.Resource{TeamID: target.OwnerTeamID, Environment: d.Environment})
	if err != nil {
		return entities.Cron{}, err
	}
	c, err := u.planned(ctx, d, target)
	if err != nil {
		return entities.Cron{}, err
	}
	if gate == authz.NeedsApproval {
		return entities.Cron{}, u.propose(ctx, a, c, Proposal{Kind: approval.KindCronCreate, Draft: &d})
	}
	return u.create(ctx, a, gate, c)
}

func (u CronsUsecase) create(ctx context.Context, a actor.Actor, gate authz.Gate, c entities.Cron) (entities.Cron, error) {
	if err := u.store.Create(ctx, c); err != nil {
		return entities.Cron{}, err
	}
	u.publish(ctx, a, gate, "cron.created", c, map[string]any{"schedule": c.Schedule, "path": c.Path})
	return c, nil
}

func (u CronsUsecase) target(ctx context.Context, d Draft) (ServiceRef, error) {
	if err := entities.ValidateName(d.Name); err != nil {
		return ServiceRef{}, err
	}
	if err := u.catalog.EnvironmentExists(ctx, d.Environment); err != nil {
		return ServiceRef{}, err
	}
	service, err := u.catalog.FindService(ctx, d.Service)
	if err == nil && service.Decommissioning {
		return ServiceRef{}, errDecommissioned
	}
	return service, err
}

func (u CronsUsecase) planned(ctx context.Context, d Draft, target ServiceRef) (entities.Cron, error) {
	c, err := newCron(d, target)
	if err != nil {
		return entities.Cron{}, err
	}
	return c, u.checkFree(ctx, c)
}

func newCron(d Draft, target ServiceRef) (entities.Cron, error) {
	payload, err := entities.NormalizePayload(d.Payload)
	if err != nil {
		return entities.Cron{}, err
	}
	now := clock.Now().UTC()
	c := entities.Cron{
		ID: uuid.NewUUID(), Name: d.Name, Environment: d.Environment, TeamID: target.OwnerTeamID, ServiceID: target.ID,
		Schedule: d.Schedule, Path: d.Path, Payload: payload, Timeout: timeoutOr(d.Timeout), CreatedAt: now, UpdatedAt: now,
	}
	return c, c.Validate()
}

func timeoutOr(timeout time.Duration) time.Duration {
	if timeout == 0 {
		return entities.DefaultTimeout
	}
	return timeout
}

func (u CronsUsecase) checkFree(ctx context.Context, c entities.Cron) error {
	_, err := u.store.Get(ctx, c.Environment, c.Name)
	if err == nil {
		return errNameTaken
	}
	if apperror.CodeOf(err) == apperror.CodeNotFound {
		return nil
	}
	return err
}

func (u CronsUsecase) Update(ctx context.Context, a actor.Actor, environment, name string, change Change) (entities.Cron, error) {
	c, gate, err := u.editable(ctx, a, environment, name)
	if err != nil {
		return entities.Cron{}, err
	}
	next, err := applied(c, change)
	if err != nil {
		return entities.Cron{}, err
	}
	if gate == authz.NeedsApproval {
		return entities.Cron{}, u.propose(ctx, a, c, Proposal{Kind: approval.KindCronUpdate, Change: &change})
	}
	if err := u.store.Save(ctx, next); err != nil {
		return entities.Cron{}, err
	}
	u.publish(ctx, a, gate, "cron.updated", next, map[string]any{"schedule": next.Schedule, "path": next.Path})
	return next, nil
}

func applied(c entities.Cron, change Change) (entities.Cron, error) {
	c.Schedule = valueOr(change.Schedule, c.Schedule)
	c.Path = valueOr(change.Path, c.Path)
	c.Timeout = valueOr(change.Timeout, c.Timeout)
	payload, err := entities.NormalizePayload(valueOr(change.Payload, c.Payload))
	if err != nil {
		return entities.Cron{}, err
	}
	c.Payload, c.UpdatedAt = payload, clock.Now().UTC()
	return c, c.Validate()
}

func valueOr[T any](changed *T, current T) T {
	if changed == nil {
		return current
	}
	return *changed
}
