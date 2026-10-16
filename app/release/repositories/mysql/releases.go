package mysql

import (
	"context"
	"errors"
	"time"

	"github.com/adrianozp/platform/app/release/entities"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	duplicateEntry = 1062
	numberAttempts = 3
)

var (
	errReleaseNotFound = apperror.NotFound("release not found").WithHint("list a service's releases with GET /api/v1/releases?service=<slug>")
	errReleaseChanged  = apperror.Conflict("the release changed meanwhile, reload and try again")
)

type ReleaseRepository struct {
	db *gorm.DB
}

func NewReleaseRepository(db *gorm.DB) ReleaseRepository {
	return ReleaseRepository{db: db}
}

func (r ReleaseRepository) Create(ctx context.Context, rel entities.Release) (entities.Release, error) {
	var err error
	for attempt := 0; attempt < numberAttempts; attempt++ {
		if rel.Number, err = r.nextNumber(ctx, rel.ServiceID); err != nil {
			return entities.Release{}, err
		}
		err = r.db.WithContext(ctx).Create(ptr(fromRelease(rel))).Error
		if !isDuplicate(err) {
			return rel, err
		}
	}
	return entities.Release{}, err
}

func (r ReleaseRepository) nextNumber(ctx context.Context, serviceID string) (int, error) {
	var highest *int
	err := r.db.WithContext(ctx).Model(&release{}).Where("service_id = ?", serviceID).
		Select("MAX(number)").Scan(&highest).Error
	if err != nil || highest == nil {
		return 1, err
	}
	return *highest + 1, nil
}

func (r ReleaseRepository) Transition(ctx context.Context, rel entities.Release, from entities.Status) error {
	result := r.db.WithContext(ctx).Model(&release{}).
		Where("id = ? AND status = ?", rel.ID, from).
		Select("status", "commit_sha", "tag", "digest", "run_url", "failure", "finished_at").
		Updates(fromRelease(rel))
	if result.Error == nil && result.RowsAffected == 0 {
		return errReleaseChanged
	}
	return result.Error
}

func (r ReleaseRepository) FindByID(ctx context.Context, id string) (entities.Release, error) {
	var model release
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Release{}, errReleaseNotFound
	}
	return model.toEntity(), err
}

func (r ReleaseRepository) ListByService(ctx context.Context, serviceID string, before, limit int) ([]entities.Release, error) {
	return r.list(belowNumber(r.db.WithContext(ctx).Where("service_id = ?", serviceID), before).Order("number DESC").Limit(limit))
}

func belowNumber(tx *gorm.DB, before int) *gorm.DB {
	if before <= 0 {
		return tx
	}
	return tx.Where("number < ?", before)
}

func (r ReleaseRepository) Unfinished(ctx context.Context, createdBefore time.Time) ([]entities.Release, error) {
	return r.list(r.db.WithContext(ctx).
		Where("status IN ? AND created_at < ?", []entities.Status{entities.StatusRequested, entities.StatusBuilding}, createdBefore).
		Clauses(clause.OrderBy{Expression: clause.Expr{SQL: "created_at"}}))
}

func (r ReleaseRepository) UnfinishedFor(ctx context.Context, serviceID string) ([]entities.Release, error) {
	return r.list(r.db.WithContext(ctx).
		Where("service_id = ? AND status IN ?", serviceID, []entities.Status{entities.StatusRequested, entities.StatusBuilding}))
}

func (r ReleaseRepository) list(tx *gorm.DB) ([]entities.Release, error) {
	var models []release
	if err := tx.Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.Release, 0, len(models))
	for _, m := range models {
		result = append(result, m.toEntity())
	}
	return result, nil
}

func ptr[T any](v T) *T { return &v }

func isDuplicate(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == duplicateEntry
}
