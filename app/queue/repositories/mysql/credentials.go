package mysql

import (
	"context"
	"errors"

	"github.com/adrianozp/platform/app/queue/entities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r QueueRepository) Credentials(ctx context.Context, serviceID, environment string) (entities.Credentials, bool, error) {
	var model credentialsModel
	err := r.db.WithContext(ctx).Where("service_id = ? AND environment = ?", serviceID, environment).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Credentials{}, false, nil
	}
	return model.toEntity(), err == nil, err
}

func (r QueueRepository) CreateCredentials(ctx context.Context, c entities.Credentials) (bool, error) {
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(ptr(fromCredentials(c)))
	return result.RowsAffected == 1, result.Error
}

func (r QueueRepository) SaveCredentials(ctx context.Context, c entities.Credentials) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(ptr(fromCredentials(c))).Error
}

func (r QueueRepository) DeleteCredentials(ctx context.Context, serviceID string) error {
	return r.db.WithContext(ctx).Where("service_id = ?", serviceID).Delete(&credentialsModel{}).Error
}

func (r QueueRepository) AllCredentials(ctx context.Context) ([]entities.Credentials, error) {
	var models []credentialsModel
	if err := r.db.WithContext(ctx).Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.Credentials, 0, len(models))
	for _, m := range models {
		result = append(result, m.toEntity())
	}
	return result, nil
}
