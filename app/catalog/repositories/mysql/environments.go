package mysql

import (
	"context"
	"errors"

	"github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/pkg/apperror"
	"gorm.io/gorm"
)

var errEnvironmentNotFound = apperror.NotFound("environment not found").WithHint("list environments with GET /api/v1/environments")

type EnvironmentRepository struct {
	db *gorm.DB
}

func NewEnvironmentRepository(db *gorm.DB) EnvironmentRepository {
	return EnvironmentRepository{db: db}
}

func (r EnvironmentRepository) List(ctx context.Context) ([]entities.Environment, error) {
	var models []environment
	if err := r.db.WithContext(ctx).Order("position").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.Environment, 0, len(models))
	for _, m := range models {
		result = append(result, m.toEntity())
	}
	return result, nil
}

func (r EnvironmentRepository) Get(ctx context.Context, slug string) (entities.Environment, error) {
	var model environment
	err := r.db.WithContext(ctx).Where("slug = ?", slug).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Environment{}, errEnvironmentNotFound
	}
	return model.toEntity(), err
}
