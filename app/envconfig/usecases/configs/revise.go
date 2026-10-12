package configs

import (
	"context"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/approval"
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

type edit struct {
	target
	current entities.Revision
	action  authz.Action
	gate    authz.Gate
}

func (u ConfigUsecase) Revise(ctx context.Context, a actor.Actor, serviceSlug, environment string, cs entities.ChangeSet) (entities.Revision, error) {
	e, err := u.editable(ctx, a, authz.EditConfig, serviceSlug, environment)
	if err != nil {
		return entities.Revision{}, err
	}
	cs.SetSecrets, cs.UnsetSecrets = nil, nil
	return u.change(ctx, a, e, approval.KindConfigRevise, cs)
}

func (u ConfigUsecase) editable(ctx context.Context, a actor.Actor, action authz.Action, serviceSlug, environment string) (edit, error) {
	t, err := u.resolve(ctx, serviceSlug, environment)
	if err != nil {
		return edit{}, err
	}
	gate, err := u.authz.Gate(ctx, a, action, t.resource())
	if err != nil {
		return edit{}, err
	}
	current, err := u.current(ctx, t)
	return edit{target: t, current: current, action: action, gate: gate}, err
}

func (u ConfigUsecase) change(ctx context.Context, a actor.Actor, e edit, kind string, cs entities.ChangeSet) (entities.Revision, error) {
	next := cs.ApplyTo(e.current.Next())
	if err := u.validate(ctx, e.target, next); err != nil {
		return entities.Revision{}, err
	}
	changes := entities.Diff(e.current, next)
	if len(changes) == 0 {
		return entities.Revision{}, errNoChanges
	}
	if e.gate == authz.NeedsApproval {
		return entities.Revision{}, u.propose(ctx, a, e, kind, cs, changes)
	}
	return u.commit(ctx, a, e, next, changes)
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

func (u ConfigUsecase) commit(ctx context.Context, a actor.Actor, e edit, next entities.Revision, changes []entities.Change) (entities.Revision, error) {
	next.ID, next.AuthorID, next.CreatedAt = uuid.NewUUID(), a.UserID, clock.Now().UTC()
	if err := u.store.Insert(ctx, next); err != nil {
		return entities.Revision{}, err
	}
	u.publisher.Publish(ctx, revisedEvent(a, e.target, next, changes))
	authz.PublishBypass(ctx, u.publisher, e.gate, a, e.action, e.resource())
	return next, nil
}

func revisedEvent(a actor.Actor, t target, rev entities.Revision, changes []entities.Change) events.Event {
	return events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: "config.revised",
		TargetKind: events.TargetConfig, TargetID: t.service.ID, TargetName: t.service.Slug,
		TeamID: t.service.OwnerTeamID, Environment: t.environment,
		Details: events.WithApproval(a, map[string]any{"revision": rev.Number, "changed": entities.ChangedKeys(changes), "message": rev.Message}),
	}
}
