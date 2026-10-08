package mysql

import (
	"context"
	"time"

	"github.com/adrianozp/platform/app/queue/entities"
)

func (r QueueRepository) AddCommand(ctx context.Context, c entities.Command) error {
	return r.db.WithContext(ctx).Create(ptr(fromCommand(c))).Error
}

func (r QueueRepository) PendingCommands(ctx context.Context) ([]entities.Command, error) {
	var models []commandModel
	if err := r.db.WithContext(ctx).Where("done_at IS NULL").Order("created_at").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.Command, 0, len(models))
	for _, m := range models {
		result = append(result, m.toEntity())
	}
	return result, nil
}

func (r QueueRepository) FinishCommand(ctx context.Context, id string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&commandModel{ID: id}).Update("done_at", at).Error
}
