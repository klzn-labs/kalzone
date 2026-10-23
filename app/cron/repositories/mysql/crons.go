package mysql

import (
	"context"
	"errors"

	"github.com/go-sql-driver/mysql"
	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"gorm.io/gorm"
)

const duplicateEntry = 1062

var (
	errNotFound  = apperror.NotFound("cron not found").WithHint("list crons with GET /api/v1/crons")
	errNameTaken = apperror.Conflict("a cron with this name already exists in this environment")
)

type CronRepository struct {
	db *gorm.DB
}

func NewCronRepository(db *gorm.DB) CronRepository {
	return CronRepository{db: db}
}

func (r CronRepository) Create(ctx context.Context, c entities.Cron) error {
	model := fromCron(c)
	err := r.db.WithContext(ctx).Create(&model).Error
	if isDuplicate(err) {
		return errNameTaken
	}
	return err
}

func (r CronRepository) Save(ctx context.Context, c entities.Cron) error {
	model := fromCron(c)
	return r.db.WithContext(ctx).Save(&model).Error
}

func (r CronRepository) SetPaused(ctx context.Context, id string, paused bool) error {
	return r.db.WithContext(ctx).Model(&cronModel{ID: id}).UpdateColumn("paused", paused).Error
}

func (r CronRepository) Get(ctx context.Context, environment, name string) (entities.Cron, error) {
	return r.find(ctx, "environment = ? AND name = ?", environment, name)
}

func (r CronRepository) GetByID(ctx context.Context, id string) (entities.Cron, error) {
	return r.find(ctx, "id = ?", id)
}

func (r CronRepository) find(ctx context.Context, query string, args ...any) (entities.Cron, error) {
	var model cronModel
	err := r.db.WithContext(ctx).Where(query, args...).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Cron{}, errNotFound
	}
	if err != nil {
		return entities.Cron{}, err
	}
	return model.toEntity(), nil
}

func (r CronRepository) List(ctx context.Context, environment, teamID string) ([]entities.Cron, error) {
	query := r.db.WithContext(ctx).Order("environment, name")
	if environment != "" {
		query = query.Where("environment = ?", environment)
	}
	if teamID != "" {
		query = query.Where("team_id = ?", teamID)
	}
	var models []cronModel
	err := query.Find(&models).Error
	return mapAll(models, cronModel.toEntity), err
}

func (r CronRepository) All(ctx context.Context) ([]entities.Cron, error) {
	return r.List(ctx, "", "")
}

func (r CronRepository) ForService(ctx context.Context, serviceID string) ([]entities.Cron, error) {
	var models []cronModel
	err := r.db.WithContext(ctx).Where("service_id = ?", serviceID).Order("environment, name").Find(&models).Error
	return mapAll(models, cronModel.toEntity), err
}

func (r CronRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("cron_id = ?", id).Delete(&runModel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("cron_id = ?", id).Delete(&commandModel{}).Error; err != nil {
			return err
		}
		return tx.Delete(&cronModel{ID: id}).Error
	})
}

func mapAll[M any, E any](models []M, convert func(M) E) []E {
	result := make([]E, 0, len(models))
	for _, m := range models {
		result = append(result, convert(m))
	}
	return result
}

func isDuplicate(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == duplicateEntry
}
