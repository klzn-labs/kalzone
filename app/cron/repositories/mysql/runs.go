package mysql

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/cron/entities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxErrorLength = 512

const pruneRuns = "DELETE FROM cron_runs WHERE cron_id = ? AND id NOT IN " +
	"(SELECT id FROM (SELECT id FROM cron_runs WHERE cron_id = ? ORDER BY scheduled_at DESC, id DESC LIMIT ?) AS kept)"

func (r CronRepository) RecordRun(ctx context.Context, run entities.Run) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var known int64
		if err := tx.Model(&cronModel{}).Where("id = ?", run.CronID).Count(&known).Error; err != nil || known == 0 {
			return err
		}
		model := fromRun(run)
		model.Error = truncated(model.Error)
		if err := tx.Clauses(clause.Insert{Modifier: "IGNORE"}).Create(&model).Error; err != nil {
			return err
		}
		return tx.Exec(pruneRuns, run.CronID, run.CronID, entities.KeptRuns).Error
	})
}

func truncated(message string) string {
	runes := []rune(message)
	if len(runes) <= maxErrorLength {
		return message
	}
	return string(runes[:maxErrorLength])
}

func (r CronRepository) Runs(ctx context.Context, cronID string, limit int) ([]entities.Run, error) {
	var models []runModel
	err := r.db.WithContext(ctx).Where("cron_id = ?", cronID).Order("scheduled_at DESC, id DESC").Limit(limit).Find(&models).Error
	return mapAll(models, runModel.toEntity), err
}

func (r CronRepository) AddCommand(ctx context.Context, c entities.Command) error {
	model := fromCommand(c)
	return r.db.WithContext(ctx).Create(&model).Error
}

func (r CronRepository) PendingCommands(ctx context.Context) ([]entities.Command, error) {
	var models []commandModel
	err := r.db.WithContext(ctx).Where("done_at IS NULL").Order("created_at").Find(&models).Error
	return mapAll(models, commandModel.toEntity), err
}

func (r CronRepository) FinishCommand(ctx context.Context, id string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&commandModel{ID: id}).Update("done_at", at).Error
}
