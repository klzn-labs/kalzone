package mysql

import (
	"context"
	"errors"
	"time"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

const (
	duplicateEntry  = 1062
	numberAttempts  = 3
	latestSucceeded = "status = 'succeeded' AND number = (SELECT MAX(m.number) FROM deployments m " +
		"WHERE m.service_id = deployments.service_id AND m.environment = deployments.environment AND m.status = 'succeeded')"
)

var (
	errDeploymentNotFound = apperror.NotFound("deployment not found").
				WithHint("list deployments with GET /api/v1/deployments?service=<slug>&env=<env>")
	errDeploymentChanged = apperror.Conflict("the deployment changed meanwhile, reload and try again")
	activeStatuses       = []entities.Status{entities.StatusPending, entities.StatusInProgress}
)

type DeploymentRepository struct {
	db *gorm.DB
}

func NewDeploymentRepository(db *gorm.DB) DeploymentRepository {
	return DeploymentRepository{db: db}
}

func (r DeploymentRepository) Create(ctx context.Context, d entities.Deployment) (entities.Deployment, error) {
	var err error
	for attempt := 0; attempt < numberAttempts; attempt++ {
		if d.Number, err = r.nextNumber(ctx, d.ServiceID, d.Environment); err != nil {
			return entities.Deployment{}, err
		}
		model := fromDeployment(d)
		err = r.db.WithContext(ctx).Create(&model).Error
		if !isDuplicate(err) {
			return d, err
		}
	}
	return entities.Deployment{}, err
}

func (r DeploymentRepository) nextNumber(ctx context.Context, serviceID, environment string) (int, error) {
	return nextNumberIn(r.db.WithContext(ctx), serviceID, environment)
}

func nextNumberIn(tx *gorm.DB, serviceID, environment string) (int, error) {
	var highest *int
	err := tx.Model(&deployment{}).Where("service_id = ? AND environment = ?", serviceID, environment).
		Select("MAX(number)").Scan(&highest).Error
	if err != nil || highest == nil {
		return 1, err
	}
	return *highest + 1, nil
}

func (r DeploymentRepository) Transition(ctx context.Context, d entities.Deployment, from entities.Status) error {
	result := r.db.WithContext(ctx).Model(&deployment{}).Where("id = ? AND status = ?", d.ID, from).
		Select("status", "agent", "message", "started_at", "finished_at", "failure").Updates(fromDeployment(d))
	if result.Error == nil && result.RowsAffected == 0 {
		return errDeploymentChanged
	}
	return result.Error
}

func (r DeploymentRepository) FindByID(ctx context.Context, id string) (entities.Deployment, error) {
	var model deployment
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Deployment{}, errDeploymentNotFound
	}
	return model.toEntity(), err
}

func (r DeploymentRepository) History(ctx context.Context, serviceID, environment string, before, limit int) ([]entities.Deployment, error) {
	tx := r.db.WithContext(ctx).Where("service_id = ? AND environment = ?", serviceID, environment)
	if before > 0 {
		tx = tx.Where("number < ?", before)
	}
	return r.list(tx.Order("number DESC").Limit(limit))
}

func (r DeploymentRepository) Active(ctx context.Context, serviceID, environment string) ([]entities.Deployment, error) {
	return r.list(r.db.WithContext(ctx).Where("service_id = ? AND environment = ? AND status IN ?", serviceID, environment, activeStatuses))
}

func (r DeploymentRepository) LatestSucceeded(ctx context.Context, serviceID, environment string) (entities.Deployment, bool, error) {
	found, err := r.list(r.db.WithContext(ctx).Where("service_id = ? AND environment = ?", serviceID, environment).Where(latestSucceeded))
	if err != nil || len(found) == 0 {
		return entities.Deployment{}, false, err
	}
	return found[0], true, nil
}

func (r DeploymentRepository) Environments(ctx context.Context, serviceID string) ([]string, error) {
	environments := []string{}
	err := r.db.WithContext(ctx).Model(&deployment{}).Distinct().Where("service_id = ?", serviceID).
		Order("environment").Pluck("environment", &environments).Error
	return environments, err
}

func (r DeploymentRepository) AllLive(ctx context.Context) ([]entities.Deployment, error) {
	return r.list(r.db.WithContext(ctx).Where(latestSucceeded).Order("service_id, environment"))
}

func (r DeploymentRepository) StaleInProgress(ctx context.Context, startedBefore time.Time) ([]entities.Deployment, error) {
	return r.list(r.db.WithContext(ctx).Where("status = ? AND started_at < ?", entities.StatusInProgress, startedBefore))
}

func (r DeploymentRepository) OldestPending(ctx context.Context) (entities.Deployment, bool, error) {
	found, err := r.list(r.db.WithContext(ctx).Where("status = ?", entities.StatusPending).Order("created_at").Limit(1))
	if err != nil || len(found) == 0 {
		return entities.Deployment{}, false, err
	}
	return found[0], true, nil
}

func (r DeploymentRepository) list(tx *gorm.DB) ([]entities.Deployment, error) {
	var models []deployment
	if err := tx.Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.Deployment, 0, len(models))
	for _, m := range models {
		result = append(result, m.toEntity())
	}
	return result, nil
}

func isDuplicate(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == duplicateEntry
}
