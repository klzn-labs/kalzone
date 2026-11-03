package mysql

import (
	"context"
	"errors"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"gorm.io/gorm"
)

var errLinkGone = apperror.NotFound("This link is no longer valid.")

type accountLink struct {
	ID           string `gorm:"primaryKey"`
	Kind         string
	TokenHash    string
	TeamID       *string
	Role         *string
	UserID       *string
	CreatedBy    *string
	CreatorLogin *string `gorm:"->"`
	CreatedAt    time.Time
	ExpiresAt    time.Time
	UsedAt       *time.Time
	RevokedAt    *time.Time
}

func (accountLink) TableName() string { return "account_links" }

func (l accountLink) toEntity() entities.AccountLink {
	return entities.AccountLink{
		ID: l.ID, Kind: entities.LinkKind(l.Kind), TokenHash: l.TokenHash,
		TeamID: stringValue(l.TeamID), Role: stringValue(l.Role), UserID: stringValue(l.UserID), CreatedBy: stringValue(l.CreatedBy),
		CreatedByLogin: stringValue(l.CreatorLogin), CreatedAt: l.CreatedAt, ExpiresAt: l.ExpiresAt, UsedAt: l.UsedAt, RevokedAt: l.RevokedAt,
	}
}

func fromAccountLink(l entities.AccountLink) accountLink {
	return accountLink{
		ID: l.ID, Kind: string(l.Kind), TokenHash: l.TokenHash,
		TeamID: stringPointer(l.TeamID), Role: stringPointer(l.Role), UserID: stringPointer(l.UserID), CreatedBy: stringPointer(l.CreatedBy),
		CreatedAt: l.CreatedAt, ExpiresAt: l.ExpiresAt, UsedAt: l.UsedAt, RevokedAt: l.RevokedAt,
	}
}

type AccountLinkRepository struct {
	db *gorm.DB
}

func NewAccountLinkRepository(db *gorm.DB) AccountLinkRepository {
	return AccountLinkRepository{db: db}
}

func (r AccountLinkRepository) Create(ctx context.Context, l entities.AccountLink) error {
	return r.db.WithContext(ctx).Create(ptr(fromAccountLink(l))).Error
}

func ptr[T any](v T) *T { return &v }

func (r AccountLinkRepository) FindByHash(ctx context.Context, hash string) (entities.AccountLink, error) {
	var model accountLink
	err := r.db.WithContext(ctx).Where("token_hash = ?", hash).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.AccountLink{}, errLinkGone
	}
	return model.toEntity(), err
}

func (r AccountLinkRepository) RevokeUnused(ctx context.Context, kind entities.LinkKind, at time.Time) error {
	return r.db.WithContext(ctx).Model(&accountLink{}).
		Where("kind = ? AND used_at IS NULL AND revoked_at IS NULL", string(kind)).
		Update("revoked_at", at).Error
}

func (r AccountLinkRepository) RevokeUnusedFor(ctx context.Context, kind entities.LinkKind, userID string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&accountLink{}).
		Where("kind = ? AND user_id = ? AND used_at IS NULL AND revoked_at IS NULL", string(kind), userID).
		Update("revoked_at", at).Error
}

func (r AccountLinkRepository) ListInvites(ctx context.Context, teamID string, now time.Time) ([]entities.AccountLink, error) {
	var models []accountLink
	err := r.db.WithContext(ctx).
		Select("account_links.*, users.login AS creator_login").
		Joins("LEFT JOIN users ON users.id = account_links.created_by").
		Where("account_links.kind = ? AND account_links.team_id = ? AND account_links.used_at IS NULL AND account_links.revoked_at IS NULL AND account_links.expires_at > ?", string(entities.LinkInvite), teamID, now).
		Order("account_links.created_at DESC, account_links.id").Find(&models).Error
	links := make([]entities.AccountLink, 0, len(models))
	for _, m := range models {
		links = append(links, m.toEntity())
	}
	return links, err
}

func (r AccountLinkRepository) Revoke(ctx context.Context, id string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&accountLink{}).
		Where("id = ? AND used_at IS NULL AND revoked_at IS NULL", id).
		Update("revoked_at", at).Error
}
