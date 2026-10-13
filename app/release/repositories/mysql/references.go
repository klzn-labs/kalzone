package mysql

import (
	"context"
	"strings"

	"github.com/adrianozp/platform/app/release/entities"
	"gorm.io/gorm"
)

var likePrefix = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)

func (r ReleaseRepository) LatestBuilt(ctx context.Context, serviceID string) (entities.Release, bool, error) {
	return r.first(r.built(ctx, serviceID))
}

func (r ReleaseRepository) ByNumber(ctx context.Context, serviceID string, number int) (entities.Release, bool, error) {
	return r.first(r.db.WithContext(ctx).Where("service_id = ? AND number = ?", serviceID, number))
}

func (r ReleaseRepository) BuiltByCommitPrefix(ctx context.Context, serviceID, prefix string, limit int) ([]entities.Release, error) {
	return r.list(r.built(ctx, serviceID).Where("commit_sha LIKE ?", likePrefix.Replace(prefix)+"%").Limit(limit))
}

func (r ReleaseRepository) BuiltByTag(ctx context.Context, serviceID, tag string) (entities.Release, bool, error) {
	return r.first(r.built(ctx, serviceID).Where("tag = ?", tag))
}

func (r ReleaseRepository) built(ctx context.Context, serviceID string) *gorm.DB {
	return r.db.WithContext(ctx).Where("service_id = ? AND status = ?", serviceID, entities.StatusBuilt).Order("number DESC")
}

func (r ReleaseRepository) first(tx *gorm.DB) (entities.Release, bool, error) {
	found, err := r.list(tx.Limit(1))
	if err != nil || len(found) == 0 {
		return entities.Release{}, false, err
	}
	return found[0], true, nil
}
