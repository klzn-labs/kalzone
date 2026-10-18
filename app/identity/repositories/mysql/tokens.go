package mysql

import (
	"context"
	"errors"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"gorm.io/gorm"
)

const duplicateEntry = 1062

var (
	errTokenNameTaken = apperror.Conflict("you already have a token with this name, pick another")
	errTokenNotFound  = apperror.NotFound("token not found")
)

type TokenRepository struct {
	db *gorm.DB
}

func NewTokenRepository(db *gorm.DB) TokenRepository {
	return TokenRepository{db: db}
}

func (r TokenRepository) Create(ctx context.Context, t entities.AccessToken) error {
	model, err := fromAccessToken(t)
	if err != nil {
		return err
	}
	err = r.db.WithContext(ctx).Create(&model).Error
	if isDuplicate(err) {
		return errTokenNameTaken
	}
	return err
}

func (r TokenRepository) FindByHash(ctx context.Context, hash string) (entities.AccessToken, error) {
	return r.findWhere(ctx, entities.ErrTokenInvalid, "token_hash = ?", hash)
}

func (r TokenRepository) FindByID(ctx context.Context, id string) (entities.AccessToken, error) {
	return r.findWhere(ctx, errTokenNotFound, "id = ?", id)
}

func (r TokenRepository) findWhere(ctx context.Context, missing error, query, value string) (entities.AccessToken, error) {
	var model accessToken
	err := r.db.WithContext(ctx).Where(query, value).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.AccessToken{}, missing
	}
	if err != nil {
		return entities.AccessToken{}, err
	}
	return model.toEntity()
}

func (r TokenRepository) ListByOwner(ctx context.Context, ownerID string) ([]entities.AccessToken, error) {
	return list(r.db.WithContext(ctx).Where("owner_id = ?", ownerID))
}

func (r TokenRepository) ListAll(ctx context.Context) ([]entities.AccessToken, error) {
	return list(r.db.WithContext(ctx))
}

func list(tx *gorm.DB) ([]entities.AccessToken, error) {
	var models []accessToken
	if err := tx.Order("created_at DESC, id").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.AccessToken, 0, len(models))
	for _, m := range models {
		t, err := m.toEntity()
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, nil
}

func (r TokenRepository) Revoke(ctx context.Context, id string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&accessToken{}).Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", at).Error
}

func (r TokenRepository) Touch(ctx context.Context, id string, at time.Time, ip string) error {
	return r.db.WithContext(ctx).Model(&accessToken{}).Where("id = ?", id).
		Updates(map[string]any{"last_used_at": at, "last_used_ip": ip}).Error
}

func isDuplicate(err error) bool {
	var mysqlErr *driver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == duplicateEntry
}
