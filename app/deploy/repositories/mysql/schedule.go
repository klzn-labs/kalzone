package mysql

import (
	"context"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"gorm.io/gorm"
)

const supersededMessage = "CONCAT('superseded before deployment #', number, ' started')"

var errStartedMeanwhile = apperror.Conflict("a deployment started meanwhile on one of the targets, nothing was deployed").
	WithHint("follow the running deployments with GET /api/v1/deployments?service=<slug>&env=<env>, then send the batch again")

var errDuplicateTarget = apperror.Invalid("each target may appear once").
	WithHint("send one entry per service and environment")

func hasDuplicateTarget(ds []entities.Deployment) bool {
	seen := make(map[[2]string]bool, len(ds))
	for _, d := range ds {
		key := [2]string{d.ServiceID, d.Environment}
		if seen[key] {
			return true
		}
		seen[key] = true
	}
	return false
}

func (r DeploymentRepository) ScheduleAll(ctx context.Context, ds []entities.Deployment) ([]entities.Deployment, error) {
	if hasDuplicateTarget(ds) {
		return nil, errDuplicateTarget
	}
	var err error
	for attempt := 0; attempt < numberAttempts; attempt++ {
		var created []entities.Deployment
		created, err = r.scheduleOnce(ctx, ds)
		if !isDuplicate(err) {
			return created, err
		}
	}
	return nil, err
}

func (r DeploymentRepository) scheduleOnce(ctx context.Context, ds []entities.Deployment) ([]entities.Deployment, error) {
	var created []entities.Deployment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		created, err = scheduleEach(tx, ds)
		return err
	})
	return created, err
}

func scheduleEach(tx *gorm.DB, ds []entities.Deployment) ([]entities.Deployment, error) {
	created := make([]entities.Deployment, 0, len(ds))
	for _, d := range ds {
		next, err := scheduleIn(tx, d)
		if err != nil {
			return nil, err
		}
		created = append(created, next)
	}
	return created, nil
}

func scheduleIn(tx *gorm.DB, d entities.Deployment) (entities.Deployment, error) {
	if err := supersedePending(tx, d); err != nil {
		return entities.Deployment{}, err
	}
	if err := nothingInProgress(tx, d); err != nil {
		return entities.Deployment{}, err
	}
	number, err := nextNumberIn(tx, d.ServiceID, d.Environment)
	if err != nil {
		return entities.Deployment{}, err
	}
	d.Number = number
	model := fromDeployment(d)
	return d, tx.Create(&model).Error
}

func supersedePending(tx *gorm.DB, d entities.Deployment) error {
	return tx.Model(&deployment{}).
		Where("service_id = ? AND environment = ? AND status = ?", d.ServiceID, d.Environment, entities.StatusPending).
		Updates(map[string]any{"status": entities.StatusCancelled, "message": gorm.Expr(supersededMessage), "finished_at": d.CreatedAt}).Error
}

func nothingInProgress(tx *gorm.DB, d entities.Deployment) error {
	var running int64
	err := tx.Model(&deployment{}).
		Where("service_id = ? AND environment = ? AND status = ?", d.ServiceID, d.Environment, entities.StatusInProgress).
		Count(&running).Error
	if err == nil && running > 0 {
		return errStartedMeanwhile
	}
	return err
}
