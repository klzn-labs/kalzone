package configs

import (
	"context"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/events"
	"github.com/adrianozp/platform/pkg/uuid"
)

var (
	errHostNotAllowed = apperror.Invalid("host must be under one of the platform's domains")
	errHostTaken      = apperror.Conflict("another service or environment already uses this host")
	errInternalTaken  = apperror.Conflict("another service is already this environment's internal service")
	errNoChanges      = apperror.Invalid("nothing changed")
)

func (u ConfigUsecase) Revise(ctx context.Context, a actor.Actor, serviceSlug, environment string, cs entities.ChangeSet) (entities.Revision, error) {
	t, current, err := u.editable(ctx, a, authz.EditConfig, serviceSlug, environment)
	if err != nil {
		return entities.Revision{}, err
	}
	next := cs.ApplyTo(current.Next())
	if err := u.validate(ctx, t, next); err != nil {
		return entities.Revision{}, err
	}
	return u.commit(ctx, a, t, current, next)
}

func (u ConfigUsecase) editable(ctx context.Context, a actor.Actor, action authz.Action, serviceSlug, environment string) (target, entities.Revision, error) {
	t, err := u.resolve(ctx, serviceSlug, environment)
	if err != nil {
		return target{}, entities.Revision{}, err
	}
	resource := authz.Resource{TeamID: t.service.OwnerTeamID, Environment: environment}
	if err := u.authz.Can(ctx, a, action, resource); err != nil {
		return target{}, entities.Revision{}, err
	}
	current, err := u.current(ctx, t)
	return t, current, err
}

func (u ConfigUsecase) validate(ctx context.Context, t target, next entities.Revision) error {
	if err := next.Validate(); err != nil {
		return err
	}
	if next.Spec.Internal {
		return u.internalFree(ctx, t)
	}
	return u.hostsFree(ctx, t, next.Spec.Hosts())
}

func (u ConfigUsecase) hostsFree(ctx context.Context, t target, hosts []string) error {
	if len(hosts) == 0 {
		return nil
	}
	for _, host := range hosts {
		if !entities.HostAllowed(host, u.allowedDomains) {
			return errHostNotAllowed
		}
	}
	taken, err := u.store.HostsTaken(ctx, hosts, t.service.ID, t.environment)
	if err != nil || !taken {
		return err
	}
	return errHostTaken
}

func (u ConfigUsecase) internalFree(ctx context.Context, t target) error {
	taken, err := u.store.InternalTaken(ctx, t.service.ID, t.environment)
	if err != nil || !taken {
		return err
	}
	return errInternalTaken
}

func (u ConfigUsecase) commit(ctx context.Context, a actor.Actor, t target, current, next entities.Revision) (entities.Revision, error) {
	changes := entities.Diff(current, next)
	if len(changes) == 0 {
		return entities.Revision{}, errNoChanges
	}
	next.ID, next.AuthorID, next.CreatedAt = uuid.NewUUID(), a.UserID, clock.Now().UTC()
	if err := u.store.Insert(ctx, next); err != nil {
		return entities.Revision{}, err
	}
	u.publisher.Publish(ctx, revisedEvent(a, t, next, changes))
	return next, nil
}

func revisedEvent(a actor.Actor, t target, rev entities.Revision, changes []entities.Change) events.Event {
	return events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: "config.revised",
		TargetKind: events.TargetConfig, TargetID: t.service.ID, TargetName: t.service.Slug,
		TeamID: t.service.OwnerTeamID, Environment: t.environment,
		Details: map[string]any{"revision": rev.Number, "changed": entities.ChangedKeys(changes), "message": rev.Message},
	}
}
