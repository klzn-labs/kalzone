package mysql

import (
	"context"
	"errors"

	"github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

const duplicateEntry = 1062

var (
	errServiceNotFound = apperror.NotFound("service not found")
	errServiceTaken    = apperror.Conflict("a service with this slug already exists or existed")
)

type ServiceRepository struct {
	db *gorm.DB
}

func NewServiceRepository(db *gorm.DB) ServiceRepository {
	return ServiceRepository{db: db}
}

func (r ServiceRepository) Create(ctx context.Context, s entities.Service) error {
	model := fromService(s)
	err := r.db.WithContext(ctx).Create(&model).Error
	if isDuplicate(err) {
		return errServiceTaken
	}
	return err
}

func (r ServiceRepository) Update(ctx context.Context, s entities.Service) error {
	return r.db.WithContext(ctx).Save(fromService(s)).Error
}

func (r ServiceRepository) FindBySlug(ctx context.Context, slug string) (entities.Service, error) {
	return r.findOne(ctx, "slug = ?", slug)
}

func (r ServiceRepository) FindByID(ctx context.Context, id string) (entities.Service, error) {
	return r.findOne(ctx, "id = ?", id)
}

func (r ServiceRepository) List(ctx context.Context, filter entities.ServiceFilter) ([]entities.Service, error) {
	if filter.OwnerTeamIDs != nil && len(filter.OwnerTeamIDs) == 0 {
		return []entities.Service{}, nil
	}
	var models []service
	if err := filtered(r.db.WithContext(ctx), filter).Order("slug").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.Service, 0, len(models))
	for _, m := range models {
		result = append(result, m.toEntity())
	}
	return result, nil
}

func filtered(tx *gorm.DB, filter entities.ServiceFilter) *gorm.DB {
	tx = tx.Where("deleted_at IS NULL")
	if filter.OwnerTeamIDs != nil {
		tx = tx.Where("owner_team_id IN ?", filter.OwnerTeamIDs)
	}
	if filter.GitHubRepo != "" {
		tx = tx.Where("github_repo = ?", filter.GitHubRepo)
	}
	return tx
}

func (r ServiceRepository) CountOwnedBy(ctx context.Context, teamID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&service{}).
		Where("owner_team_id = ? AND deleted_at IS NULL", teamID).Count(&count).Error
	return count, err
}

func (r ServiceRepository) findOne(ctx context.Context, query string, arg any) (entities.Service, error) {
	var model service
	err := r.db.WithContext(ctx).Where("deleted_at IS NULL").Where(query, arg).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Service{}, errServiceNotFound
	}
	return model.toEntity(), err
}

func isDuplicate(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == duplicateEntry
}
