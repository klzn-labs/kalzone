package mysql

import (
	"context"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"gorm.io/gorm"
)

const duplicateEntry = 1062

var (
	errNotFound  = apperror.NotFound("database not found").WithHint("list databases with GET /api/v1/databases")
	errNameTaken = apperror.Conflict("a database with this name already exists in this environment")
)

type DatabaseRepository struct {
	db *gorm.DB
}

func NewDatabaseRepository(db *gorm.DB) DatabaseRepository {
	return DatabaseRepository{db: db}
}

func (r DatabaseRepository) Create(ctx context.Context, d entities.Database) error {
	model := fromDatabase(d)
	err := r.db.WithContext(ctx).Create(&model).Error
	if isDuplicate(err) {
		return errNameTaken
	}
	return err
}

func (r DatabaseRepository) Get(ctx context.Context, environment, name string) (entities.Database, error) {
	return r.find(ctx, "environment = ? AND name = ?", environment, name)
}

func (r DatabaseRepository) GetByID(ctx context.Context, id string) (entities.Database, error) {
	return r.find(ctx, "id = ?", id)
}

func (r DatabaseRepository) find(ctx context.Context, query string, args ...any) (entities.Database, error) {
	var model databaseModel
	err := r.db.WithContext(ctx).Where(query, args...).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Database{}, errNotFound
	}
	if err != nil {
		return entities.Database{}, err
	}
	return model.toEntity(), nil
}

func (r DatabaseRepository) List(ctx context.Context, environment, teamID string) ([]entities.Database, error) {
	query := r.db.WithContext(ctx).Order("environment, name")
	if environment != "" {
		query = query.Where("environment = ?", environment)
	}
	if teamID != "" {
		query = query.Where("team_id = ?", teamID)
	}
	var models []databaseModel
	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}
	return mapAll(models, databaseModel.toEntity), nil
}

func (r DatabaseRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("database_id = ?", id).Delete(&attachmentModel{}).Error; err != nil {
			return err
		}
		return tx.Delete(&databaseModel{ID: id}).Error
	})
}

func (r DatabaseRepository) Attach(ctx context.Context, a entities.Attachment) error {
	model := fromAttachment(a)
	return r.db.WithContext(ctx).Save(&model).Error
}

func (r DatabaseRepository) Detach(ctx context.Context, databaseID, serviceID string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&attachmentModel{}).
		Where("database_id = ? AND service_id = ?", databaseID, serviceID).
		Update("detached_at", at).Error
}

func (r DatabaseRepository) Attachments(ctx context.Context, databaseID string) ([]entities.Attachment, error) {
	var models []attachmentModel
	err := r.db.WithContext(ctx).Where("database_id = ?", databaseID).Order("service_id").Find(&models).Error
	return mapAll(models, attachmentModel.toEntity), err
}

func (r DatabaseRepository) ActiveFor(ctx context.Context, serviceID, environment string) (entities.Database, bool, error) {
	var models []databaseModel
	err := r.db.WithContext(ctx).
		Joins("JOIN database_attachments a ON a.database_id = databases.id").
		Where("a.service_id = ? AND a.environment = ? AND a.detached_at IS NULL", serviceID, environment).
		Limit(1).Find(&models).Error
	if err != nil || len(models) == 0 {
		return entities.Database{}, false, err
	}
	return models[0].toEntity(), true, nil
}

func (r DatabaseRepository) ForgetDrained(ctx context.Context, databaseID, serviceID string) error {
	return r.db.WithContext(ctx).
		Where("database_id = ? AND service_id = ? AND detached_at IS NOT NULL", databaseID, serviceID).
		Delete(&attachmentModel{}).Error
}

func (r DatabaseRepository) DetachService(ctx context.Context, serviceID string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&attachmentModel{}).
		Where("service_id = ? AND detached_at IS NULL", serviceID).
		Update("detached_at", at).Error
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
