package mysql

import (
	"context"
	"errors"

	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

const duplicateEntry = 1062

var (
	errQueueNotFound = apperror.NotFound("queue not found")
	errNameTaken     = apperror.Conflict("a queue with this name already exists in this environment")
	errConcurrent    = apperror.Conflict("the queue changed while you were editing it, reload and try again")
)

type QueueRepository struct {
	db *gorm.DB
}

func NewQueueRepository(db *gorm.DB) QueueRepository {
	return QueueRepository{db: db}
}

func (r QueueRepository) Create(ctx context.Context, q entities.Queue, v entities.Version) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(ptr(fromQueue(q))).Error; err != nil {
			return err
		}
		return writeVersion(tx, q, v)
	})
	if isDuplicate(err) {
		return errNameTaken
	}
	return err
}

func (r QueueRepository) InsertVersion(ctx context.Context, q entities.Queue, v entities.Version) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := writeVersion(tx, q, v); err != nil {
			return err
		}
		return tx.Model(&queueModel{ID: q.ID}).Updates(map[string]any{"version": q.Version, "updated_at": q.UpdatedAt}).Error
	})
	if isDuplicate(err) {
		return errConcurrent
	}
	return err
}

func writeVersion(tx *gorm.DB, q entities.Queue, v entities.Version) error {
	model, err := fromVersion(v)
	if err != nil {
		return err
	}
	if err := tx.Create(&model).Error; err != nil {
		return err
	}
	if err := tx.Where("queue_id = ?", q.ID).Delete(&publisherModel{}).Error; err != nil {
		return err
	}
	return createPublishers(tx, q)
}

func createPublishers(tx *gorm.DB, q entities.Queue) error {
	if len(q.Publishers) == 0 {
		return nil
	}
	return tx.Create(publisherModels(q.ID, q.Publishers)).Error
}

func (r QueueRepository) SetPaused(ctx context.Context, queueID string, paused bool) error {
	return r.db.WithContext(ctx).Model(&queueModel{ID: queueID}).Update("paused", paused).Error
}

func (r QueueRepository) Get(ctx context.Context, environment, name string) (entities.Queue, error) {
	var model queueModel
	err := r.db.WithContext(ctx).Where("environment = ? AND name = ?", environment, name).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Queue{}, errQueueNotFound
	}
	if err != nil {
		return entities.Queue{}, err
	}
	return r.hydrate(ctx, model)
}

func (r QueueRepository) List(ctx context.Context, environment, teamID string) ([]entities.Queue, error) {
	query := r.db.WithContext(ctx).Order("environment, name")
	if environment != "" {
		query = query.Where("environment = ?", environment)
	}
	if teamID != "" {
		query = query.Where("team_id = ?", teamID)
	}
	var models []queueModel
	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}
	return r.hydrateAll(ctx, models)
}

func (r QueueRepository) All(ctx context.Context) ([]entities.Queue, error) {
	return r.List(ctx, "", "")
}

func (r QueueRepository) Delete(ctx context.Context, queueID string, deletion entities.Command) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return removeQueue(tx, queueID, deletion) })
}

func removeQueue(tx *gorm.DB, queueID string, deletion entities.Command) error {
	for _, model := range []any{&publisherModel{}, &versionModel{}, &commandModel{}} {
		if err := tx.Where("queue_id = ?", queueID).Delete(model).Error; err != nil {
			return err
		}
	}
	if err := tx.Delete(&queueModel{ID: queueID}).Error; err != nil {
		return err
	}
	return tx.Create(ptr(fromCommand(deletion))).Error
}

func (r QueueRepository) Versions(ctx context.Context, queueID string, before, limit int) ([]entities.Version, error) {
	var models []versionModel
	tx := r.db.WithContext(ctx).Where("queue_id = ?", queueID)
	if before > 0 {
		tx = tx.Where("number < ?", before)
	}
	if err := tx.Order("number DESC").Limit(limit).Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.Version, 0, len(models))
	for _, m := range models {
		v, err := m.toEntity()
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}

func (r QueueRepository) hydrateAll(ctx context.Context, models []queueModel) ([]entities.Queue, error) {
	result := make([]entities.Queue, 0, len(models))
	for _, m := range models {
		q, err := r.hydrate(ctx, m)
		if err != nil {
			return nil, err
		}
		result = append(result, q)
	}
	return result, nil
}

func (r QueueRepository) hydrate(ctx context.Context, m queueModel) (entities.Queue, error) {
	var version versionModel
	err := r.db.WithContext(ctx).Where("queue_id = ? AND number = ?", m.ID, m.Version).Take(&version).Error
	if err != nil {
		return entities.Queue{}, err
	}
	v, err := version.toEntity()
	if err != nil {
		return entities.Queue{}, err
	}
	var publishers []string
	err = r.db.WithContext(ctx).Model(&publisherModel{}).Where("queue_id = ?", m.ID).Order("service_id").Pluck("service_id", &publishers).Error
	return m.toEntity(v.Settings, publishers), err
}

func isDuplicate(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == duplicateEntry
}

func ptr[T any](v T) *T {
	return &v
}
