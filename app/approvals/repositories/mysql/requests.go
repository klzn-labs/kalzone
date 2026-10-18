package mysql

import (
	"context"
	"errors"
	"time"

	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"gorm.io/gorm"
)

var (
	errRequestNotFound = apperror.NotFound("change request not found").WithHint("list change requests with GET /api/v1/requests")
	errRequestChanged  = apperror.Conflict("this request was already decided")
)

type RequestRepository struct {
	db *gorm.DB
}

func NewRequestRepository(db *gorm.DB) RequestRepository {
	return RequestRepository{db: db}
}

func (r RequestRepository) Create(ctx context.Context, req entities.Request) error {
	model := fromRequest(req)
	return r.db.WithContext(ctx).Create(&model).Error
}

func (r RequestRepository) Get(ctx context.Context, id string) (entities.Request, error) {
	var model changeRequest
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Request{}, errRequestNotFound
	}
	return model.toEntity(), err
}

func (r RequestRepository) List(ctx context.Context, f entities.Filter) ([]entities.Request, error) {
	tx := r.db.WithContext(ctx)
	if f.Status != "" {
		tx = tx.Where("status = ?", f.Status)
	}
	if len(f.TeamIDs) > 0 {
		tx = tx.Where("team_id IN ?", f.TeamIDs)
	}
	if f.ServiceID != "" {
		tx = tx.Where("service_id = ?", f.ServiceID)
	}
	return find(before(tx, f).Order("created_at DESC, id DESC").Limit(f.Limit))
}

func before(tx *gorm.DB, f entities.Filter) *gorm.DB {
	switch {
	case f.Before == nil:
		return tx
	case f.BeforeID == "":
		return tx.Where("created_at < ?", *f.Before)
	}
	return tx.Where("created_at < ? OR (created_at = ? AND id < ?)", *f.Before, *f.Before, f.BeforeID)
}

func (r RequestRepository) PendingFor(ctx context.Context, serviceID, environment, subject string) ([]entities.Request, error) {
	return find(r.db.WithContext(ctx).Where("service_id = ? AND environment = ? AND subject = ? AND status = ?",
		serviceID, environment, subject, entities.StatusPending).Order("created_at, id"))
}

func (r RequestRepository) Overdue(ctx context.Context, now time.Time) ([]entities.Request, error) {
	return find(r.db.WithContext(ctx).Where("(status = ? AND expires_at <= ?) OR (status = ? AND decided_at <= ?)",
		entities.StatusPending, now, entities.StatusApproved, now.Add(-entities.ExecutionLimit)).Order("created_at, id"))
}

func (r RequestRepository) Transition(ctx context.Context, req entities.Request, from entities.Status) error {
	result := r.db.WithContext(ctx).Model(&changeRequest{}).Where("id = ? AND status = ?", req.ID, from).
		Select("status", "decided_by", "decider_login", "decided_at", "reason", "result", "error_code", "error_message", "finished_at").
		Updates(fromRequest(req))
	if result.Error == nil && result.RowsAffected == 0 {
		return errRequestChanged
	}
	return result.Error
}

func find(tx *gorm.DB) ([]entities.Request, error) {
	var models []changeRequest
	if err := tx.Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.Request, 0, len(models))
	for _, m := range models {
		result = append(result, m.toEntity())
	}
	return result, nil
}
