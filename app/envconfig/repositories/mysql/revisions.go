package mysql

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/go-sql-driver/mysql"
	"github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"gorm.io/gorm"
)

const duplicateEntry = 1062

const currentRevisionOfPair = "number = (SELECT MAX(m.number) FROM config_revisions m " +
	"WHERE m.service_id = config_revisions.service_id AND m.environment = config_revisions.environment)"

var errRevisionNotFound = apperror.NotFound("config revision not found")

var errConcurrentRevision = apperror.Conflict("the config changed while you were editing it, reload and try again")

type ConfigRepository struct {
	db *gorm.DB
}

func NewConfigRepository(db *gorm.DB) ConfigRepository {
	return ConfigRepository{db: db}
}

func (r ConfigRepository) Latest(ctx context.Context, serviceID, environment string) (entities.Revision, bool, error) {
	var model revision
	err := r.db.WithContext(ctx).Where("service_id = ? AND environment = ?", serviceID, environment).
		Order("number DESC").Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Revision{}, false, nil
	}
	if err != nil {
		return entities.Revision{}, false, err
	}
	result, err := model.toEntity()
	return result, err == nil, err
}

func (r ConfigRepository) ByNumber(ctx context.Context, serviceID, environment string, number int) (entities.Revision, error) {
	var model revision
	err := r.db.WithContext(ctx).Where("service_id = ? AND environment = ? AND number = ?", serviceID, environment, number).
		Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Revision{}, errRevisionNotFound
	}
	if err != nil {
		return entities.Revision{}, err
	}
	return model.toEntity()
}

func (r ConfigRepository) History(ctx context.Context, serviceID, environment string, before, limit int) ([]entities.Revision, error) {
	var models []revision
	tx := r.db.WithContext(ctx).Where("service_id = ? AND environment = ?", serviceID, environment)
	if before > 0 {
		tx = tx.Where("number < ?", before)
	}
	if err := tx.Order("number DESC").Limit(limit).Find(&models).Error; err != nil {
		return nil, err
	}
	return toRevisions(models)
}

func (r ConfigRepository) Insert(ctx context.Context, rev entities.Revision) error {
	model, err := fromRevision(rev)
	if err != nil {
		return err
	}
	err = r.db.WithContext(ctx).Create(&model).Error
	if isDuplicate(err) {
		return errConcurrentRevision
	}
	return err
}

func (r ConfigRepository) HostOwners(ctx context.Context, hosts []string, serviceID, environment string) ([]string, error) {
	names, err := json.Marshal(hosts)
	if err != nil {
		return nil, err
	}
	owners := []string{}
	err = r.db.WithContext(ctx).Model(&revision{}).Distinct().
		Where("host IN ? OR JSON_OVERLAPS(JSON_EXTRACT(spec, '$.aliases'), CAST(? AS JSON))", hosts, string(names)).
		Where("NOT (service_id = ? AND environment = ?)", serviceID, environment).
		Where(currentRevisionOfPair).Pluck("service_id", &owners).Error
	return owners, err
}

func (r ConfigRepository) InternalOwners(ctx context.Context, serviceID, environment string) ([]string, error) {
	owners := []string{}
	err := r.db.WithContext(ctx).Model(&revision{}).Distinct().
		Where("environment = ? AND service_id <> ?", environment, serviceID).
		Where("JSON_EXTRACT(spec, '$.internal') = CAST('true' AS JSON)").
		Where(currentRevisionOfPair).Pluck("service_id", &owners).Error
	return owners, err
}

func toRevisions(models []revision) ([]entities.Revision, error) {
	result := make([]entities.Revision, 0, len(models))
	for _, m := range models {
		rev, err := m.toEntity()
		if err != nil {
			return nil, err
		}
		result = append(result, rev)
	}
	return result, nil
}

func isDuplicate(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == duplicateEntry
}
