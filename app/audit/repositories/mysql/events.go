package mysql

import (
	"context"

	"github.com/klzn-labs/kalzone/app/audit/entities"
	"gorm.io/gorm"
)

type EventRepository struct {
	db *gorm.DB
}

func NewEventRepository(db *gorm.DB) EventRepository {
	return EventRepository{db: db}
}

func (r EventRepository) Insert(ctx context.Context, e entities.Event) error {
	model, err := fromEvent(e)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(&model).Error
}

func (r EventRepository) List(ctx context.Context, filter entities.Filter) ([]entities.Event, error) {
	var models []auditEvent
	err := filtered(r.db.WithContext(ctx), filter).Order("at DESC, id DESC").Limit(filter.Limit).Find(&models).Error
	if err != nil {
		return nil, err
	}
	result := make([]entities.Event, 0, len(models))
	for _, m := range models {
		event, err := m.toEntity()
		if err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, nil
}

func filtered(tx *gorm.DB, f entities.Filter) *gorm.DB {
	tx = whereSet(tx, "team_id = ?", f.TeamID)
	tx = whereSet(tx, "target_id = ?", f.TargetID)
	tx = whereSet(tx, "actor_id = ?", f.ActorID)
	tx = whereSet(tx, "token_id = ?", f.TokenID)
	tx = whereSet(tx, "environment = ?", f.Environment)
	return before(visible(tx, f), f)
}

func visible(tx *gorm.DB, f entities.Filter) *gorm.DB {
	if f.AllTeams {
		return tx
	}
	return tx.Where("team_id IN ? OR (team_id = '' AND actor_id = ?)", f.TeamIDs, f.ViewerID)
}

func before(tx *gorm.DB, f entities.Filter) *gorm.DB {
	switch {
	case f.Before == nil:
		return tx
	case f.BeforeID == "":
		return tx.Where("at < ?", *f.Before)
	}
	return tx.Where("at < ? OR (at = ? AND id < ?)", *f.Before, *f.Before, f.BeforeID)
}

func whereSet(tx *gorm.DB, query, value string) *gorm.DB {
	if value == "" {
		return tx
	}
	return tx.Where(query, value)
}
