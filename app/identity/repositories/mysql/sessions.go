package mysql

import (
	"context"
	"errors"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"gorm.io/gorm"
)

var errSessionNotFound = apperror.New(apperror.CodeUnauthenticated, "session not found")

type SessionRepository struct {
	db *gorm.DB
}

func NewSessionRepository(db *gorm.DB) SessionRepository {
	return SessionRepository{db: db}
}

func (r SessionRepository) Create(ctx context.Context, s entities.Session) error {
	return r.db.WithContext(ctx).Create(fromSession(s)).Error
}

func (r SessionRepository) Find(ctx context.Context, tokenHash string) (entities.Session, error) {
	var model session
	err := r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Session{}, errSessionNotFound
	}
	return model.toEntity(), err
}

func (r SessionRepository) Extend(ctx context.Context, tokenHash string, expiresAt time.Time) error {
	return r.db.WithContext(ctx).Model(&session{}).Where("token_hash = ?", tokenHash).Update("expires_at", expiresAt).Error
}

func (r SessionRepository) Delete(ctx context.Context, tokenHash string) error {
	return r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).Delete(&session{}).Error
}

func (r SessionRepository) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	res := r.db.WithContext(ctx).Where("expires_at <= ?", now).Delete(&session{})
	return int(res.RowsAffected), res.Error
}
