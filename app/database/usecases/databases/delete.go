package databases

import (
	"context"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

var (
	errAttached     = apperror.Conflict("the database still has attached services")
	errStillRunning = apperror.Conflict("services still run with this database's credentials")
)

func (u DatabasesUsecase) Delete(ctx context.Context, a actor.Actor, environment, name string) error {
	db, gate, err := u.editable(ctx, a, environment, name)
	if err != nil {
		return err
	}
	if err := u.clearAttachments(ctx, db); err != nil {
		return err
	}
	if gate == authz.NeedsApproval {
		return u.propose(ctx, a, db, Proposal{Kind: approval.KindDatabaseDelete})
	}
	return u.drop(ctx, a, gate, db)
}

func (u DatabasesUsecase) drop(ctx context.Context, a actor.Actor, gate authz.Gate, db entities.Database) error {
	if db.Adopted {
		return u.release(ctx, a, gate, db)
	}
	if err := u.provisioner.Drop(ctx, db.Schema, db.Username); err != nil {
		return err
	}
	if err := u.store.Delete(ctx, db.ID); err != nil {
		return err
	}
	u.publish(ctx, a, gate, "database.deleted", db, map[string]any{"schema": db.Schema})
	return nil
}

func (u DatabasesUsecase) release(ctx context.Context, a actor.Actor, gate authz.Gate, db entities.Database) error {
	if err := u.provisioner.Release(ctx, db.Username); err != nil {
		return err
	}
	if err := u.store.Delete(ctx, db.ID); err != nil {
		return err
	}
	u.publish(ctx, a, gate, "database.released", db, map[string]any{"schema": db.Schema, "data": "kept"})
	return nil
}

func (u DatabasesUsecase) clearAttachments(ctx context.Context, db entities.Database) error {
	attachments, err := u.store.Attachments(ctx, db.ID)
	if err != nil {
		return err
	}
	if err := u.refuse(ctx, errAttached, "detach these first: ", activeIn(attachments)); err != nil {
		return err
	}
	drained, running, err := u.splitDetached(ctx, db, attachments)
	if err != nil {
		return err
	}
	if err := u.refuse(ctx, errStillRunning, "redeploy or remove these first: ", running); err != nil {
		return err
	}
	return u.forget(ctx, db, drained)
}

func activeIn(attachments []entities.Attachment) []string {
	ids := []string{}
	for _, a := range attachments {
		if a.Active() {
			ids = append(ids, a.ServiceID)
		}
	}
	return ids
}

func (u DatabasesUsecase) splitDetached(ctx context.Context, db entities.Database, attachments []entities.Attachment) ([]string, []string, error) {
	drained, running := []string{}, []string{}
	for _, a := range attachments {
		if a.Active() {
			continue
		}
		runs, err := u.running.RunsSince(ctx, a.ServiceID, db.Environment, *a.DetachedAt)
		if err != nil {
			return nil, nil, err
		}
		if runs {
			running = append(running, a.ServiceID)
		} else {
			drained = append(drained, a.ServiceID)
		}
	}
	return drained, running, nil
}

func (u DatabasesUsecase) refuse(ctx context.Context, conflict *apperror.AppError, hint string, serviceIDs []string) error {
	if len(serviceIDs) == 0 {
		return nil
	}
	slugs, err := u.slugsOf(ctx, serviceIDs)
	if err != nil {
		return err
	}
	return conflict.WithHint(hint + slugs)
}

func (u DatabasesUsecase) forget(ctx context.Context, db entities.Database, serviceIDs []string) error {
	for _, id := range serviceIDs {
		if err := u.store.ForgetDrained(ctx, db.ID, id); err != nil {
			return err
		}
	}
	return nil
}
