package databases

import (
	"context"
	"slices"
	"time"

	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

type AttachmentView struct {
	Service    string
	AttachedAt time.Time
	DetachedAt *time.Time
	Draining   bool
}

var errNotVisible = apperror.NotFound("database not found").WithHint("list databases with GET /api/v1/databases")

func (u DatabasesUsecase) GetFor(ctx context.Context, a actor.Actor, environment, name string) (entities.Database, error) {
	db, err := u.store.Get(ctx, environment, name)
	if err != nil {
		return entities.Database{}, err
	}
	if !a.Sees(db.TeamID) {
		return entities.Database{}, errNotVisible
	}
	return db, nil
}

func (u DatabasesUsecase) ListFor(ctx context.Context, a actor.Actor, environment, teamSlug string) ([]entities.Database, error) {
	list, err := u.list(ctx, environment, teamSlug)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list, func(db entities.Database) bool { return !a.Sees(db.TeamID) }), nil
}

func (u DatabasesUsecase) list(ctx context.Context, environment, teamSlug string) ([]entities.Database, error) {
	if teamSlug == "" {
		return u.store.List(ctx, environment, "")
	}
	teamID, err := u.catalog.FindTeam(ctx, teamSlug)
	if err != nil {
		return nil, err
	}
	return u.store.List(ctx, environment, teamID)
}

func (u DatabasesUsecase) AttachmentsFor(ctx context.Context, a actor.Actor, environment, name string) ([]AttachmentView, error) {
	db, err := u.GetFor(ctx, a, environment, name)
	if err != nil {
		return nil, err
	}
	attachments, err := u.store.Attachments(ctx, db.ID)
	if err != nil {
		return nil, err
	}
	views := make([]AttachmentView, 0, len(attachments))
	for _, at := range attachments {
		view, err := u.viewOf(ctx, at)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (u DatabasesUsecase) viewOf(ctx context.Context, at entities.Attachment) (AttachmentView, error) {
	slug, err := u.slugOf(ctx, at.ServiceID)
	if err != nil || at.Active() {
		return AttachmentView{Service: slug, AttachedAt: at.AttachedAt}, err
	}
	draining, err := u.running.RunsSince(ctx, at.ServiceID, at.Environment, *at.DetachedAt)
	return AttachmentView{Service: slug, AttachedAt: at.AttachedAt, DetachedAt: at.DetachedAt, Draining: draining}, err
}

func (u DatabasesUsecase) All(ctx context.Context) ([]entities.Database, error) {
	return u.store.List(ctx, "", "")
}
