package databases

import (
	"context"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

const serviceDeleted = "the service was deleted"

var (
	errOtherAttached = apperror.Conflict("the service already has a database attached in this environment")
	errNotAttached   = apperror.NotFound("the service is not attached to this database")
)

func (u DatabasesUsecase) Attach(ctx context.Context, a actor.Actor, environment, name, serviceSlug string) (entities.Database, error) {
	db, svc, gate, err := u.attachable(ctx, a, environment, name, serviceSlug)
	if err != nil {
		return entities.Database{}, err
	}
	current, attached, err := u.store.ActiveFor(ctx, svc.ID, environment)
	if err != nil || (attached && current.ID == db.ID) {
		return db, err
	}
	if attached {
		return entities.Database{}, errOtherAttached.WithHint("detach " + current.Name + " first")
	}
	if gate == authz.NeedsApproval {
		return entities.Database{}, u.propose(ctx, a, db, Proposal{Kind: approval.KindDatabaseAttach, Service: svc.Slug})
	}
	attachment := entities.Attachment{DatabaseID: db.ID, ServiceID: svc.ID, Environment: environment, AttachedAt: clock.Now().UTC()}
	if err := u.store.Attach(ctx, attachment); err != nil {
		return entities.Database{}, err
	}
	u.publish(ctx, a, gate, "database.attached", db, nextDeploy(svc.Slug))
	return db, nil
}

func (u DatabasesUsecase) Detach(ctx context.Context, a actor.Actor, environment, name, serviceSlug string) (entities.Database, error) {
	db, svc, gate, err := u.attachable(ctx, a, environment, name, serviceSlug)
	if err != nil {
		return entities.Database{}, err
	}
	if err := u.requireAttached(ctx, db, svc); err != nil {
		return entities.Database{}, err
	}
	if gate == authz.NeedsApproval {
		return entities.Database{}, u.propose(ctx, a, db, Proposal{Kind: approval.KindDatabaseDetach, Service: svc.Slug})
	}
	if err := u.store.Detach(ctx, db.ID, svc.ID, clock.Now().UTC()); err != nil {
		return entities.Database{}, err
	}
	u.publish(ctx, a, gate, "database.detached", db, nextDeploy(svc.Slug))
	return db, nil
}

func (u DatabasesUsecase) requireAttached(ctx context.Context, db entities.Database, svc ServiceRef) error {
	current, attached, err := u.store.ActiveFor(ctx, svc.ID, db.Environment)
	if err != nil {
		return err
	}
	if !attached {
		return errNotAttached
	}
	if current.ID != db.ID {
		return errNotAttached.WithHint(svc.Slug + " uses database " + current.Name)
	}
	return nil
}

func (u DatabasesUsecase) attachable(ctx context.Context, a actor.Actor, environment, name, serviceSlug string) (entities.Database, ServiceRef, authz.Gate, error) {
	db, gate, err := u.editable(ctx, a, environment, name)
	if err != nil {
		return entities.Database{}, ServiceRef{}, "", err
	}
	svc, err := u.catalog.FindService(ctx, serviceSlug)
	if err != nil {
		return entities.Database{}, ServiceRef{}, "", err
	}
	if _, err := u.authz.Gate(ctx, a, authz.EditSecrets, authz.Resource{TeamID: svc.OwnerTeamID, Environment: environment}); err != nil {
		return entities.Database{}, ServiceRef{}, "", err
	}
	return db, svc, gate, nil
}

func nextDeploy(serviceSlug string) map[string]any {
	return map[string]any{"service": serviceSlug, "takes_effect": "next deploy"}
}

func (u DatabasesUsecase) DetachService(ctx context.Context, a actor.Actor, serviceID string) error {
	attached, err := u.AttachedTo(ctx, serviceID)
	if err != nil || len(attached) == 0 {
		return err
	}
	if err := u.store.DetachService(ctx, serviceID, clock.Now().UTC()); err != nil {
		return err
	}
	for _, db := range attached {
		u.publish(ctx, a, authz.Open, "database.detached", db, map[string]any{"service_id": serviceID, "reason": serviceDeleted})
	}
	return nil
}

func (u DatabasesUsecase) AttachedTo(ctx context.Context, serviceID string) ([]entities.Database, error) {
	all, err := u.store.List(ctx, "", "")
	if err != nil {
		return nil, err
	}
	attached := []entities.Database{}
	for _, environment := range environmentsOf(all) {
		db, found, err := u.store.ActiveFor(ctx, serviceID, environment)
		if err != nil {
			return nil, err
		}
		if found {
			attached = append(attached, db)
		}
	}
	return attached, nil
}

func environmentsOf(all []entities.Database) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, db := range all {
		if !seen[db.Environment] {
			seen[db.Environment] = true
			result = append(result, db.Environment)
		}
	}
	return result
}
