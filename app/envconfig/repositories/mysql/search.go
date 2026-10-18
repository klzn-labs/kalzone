package mysql

import (
	"context"
	"strings"

	"github.com/klzn-labs/kalzone/app/envconfig/entities"
	"gorm.io/gorm"
)

const (
	searchColumns = "service_id, environment, number, JSON_UNQUOTE(JSON_EXTRACT(env, ?)) AS value, JSON_CONTAINS_PATH(secrets, 'one', ?) AS secret"
	searchHolds   = "(JSON_CONTAINS_PATH(env, 'one', ?) OR JSON_CONTAINS_PATH(secrets, 'one', ?))"
)

var jsonKey = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

type match struct {
	ServiceID   string
	Environment string
	Number      int
	Value       *string
	Secret      bool
}

func (m match) toEntity() entities.Match {
	if m.Secret || m.Value == nil {
		return entities.Match{ServiceID: m.ServiceID, Environment: m.Environment, Revision: m.Number, Kind: entities.MatchSecret}
	}
	return entities.Match{ServiceID: m.ServiceID, Environment: m.Environment, Revision: m.Number, Kind: entities.MatchEnv, Value: *m.Value}
}

func (r ConfigRepository) Search(ctx context.Context, key, environment string) ([]entities.Match, error) {
	path := `$."` + jsonKey.Replace(key) + `"`
	tx := r.db.WithContext(ctx).Model(&revision{}).Select(searchColumns, path, path).
		Where(searchHolds, path, path).Where(currentRevisionOfPair)
	var rows []match
	if err := inEnvironment(tx, environment).Order("service_id, environment").Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]entities.Match, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toEntity())
	}
	return result, nil
}

func inEnvironment(tx *gorm.DB, environment string) *gorm.DB {
	if environment == "" {
		return tx
	}
	return tx.Where("environment = ?", environment)
}
