package mysql

import (
	"context"
	"errors"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChangelogRepository struct {
	db *gorm.DB
}

func NewChangelogRepository(db *gorm.DB) ChangelogRepository {
	return ChangelogRepository{db: db}
}

func (r ChangelogRepository) Save(ctx context.Context, c entities.Changelog) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(ptr(fromChangelog(c))).Error
}

func (r ChangelogRepository) Find(ctx context.Context, releaseID string) (entities.Changelog, bool, error) {
	return r.first(r.db.WithContext(ctx).Where("release_id = ?", releaseID))
}

func (r ChangelogRepository) FindMany(ctx context.Context, releaseIDs []string) (map[string]entities.Changelog, error) {
	found, err := r.list(r.db.WithContext(ctx).Where("release_id IN ?", append(releaseIDs, "")))
	result := make(map[string]entities.Changelog, len(found))
	for _, c := range found {
		result[c.ReleaseID] = c
	}
	return result, err
}

func (r ChangelogRepository) Pending(ctx context.Context) ([]entities.Changelog, error) {
	return r.list(r.db.WithContext(ctx).Where("state = ?", entities.ChangelogPending).Order("updated_at"))
}

func (r ChangelogRepository) ByPair(ctx context.Context, baseSHA, headSHA string) (entities.Changelog, bool, error) {
	done := []entities.ChangelogState{entities.ChangelogPending, entities.ChangelogFailed}
	return r.first(r.db.WithContext(ctx).Where("base_sha = ? AND head_sha = ? AND state NOT IN ?", baseSHA, headSHA, done))
}

func (r ChangelogRepository) first(tx *gorm.DB) (entities.Changelog, bool, error) {
	var model changelog
	err := tx.Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Changelog{}, false, nil
	}
	return model.toEntity(), err == nil, err
}

func (r ChangelogRepository) list(tx *gorm.DB) ([]entities.Changelog, error) {
	var models []changelog
	if err := tx.Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.Changelog, 0, len(models))
	for _, m := range models {
		result = append(result, m.toEntity())
	}
	return result, nil
}
