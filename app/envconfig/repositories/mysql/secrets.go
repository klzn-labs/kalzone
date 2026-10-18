package mysql

import (
	"context"

	"github.com/klzn-labs/kalzone/app/envconfig/entities"
)

func (r ConfigRepository) InsertSecretVersion(ctx context.Context, v entities.SecretVersion) error {
	model := fromSecretVersion(v)
	return r.db.WithContext(ctx).Create(&model).Error
}

func (r ConfigRepository) SecretVersions(ctx context.Context, ids []string) ([]entities.SecretVersion, error) {
	if len(ids) == 0 {
		return []entities.SecretVersion{}, nil
	}
	var models []secretVersion
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Order("name").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.SecretVersion, 0, len(models))
	for _, m := range models {
		result = append(result, m.toEntity())
	}
	return result, nil
}
