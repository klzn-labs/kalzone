package mysql

import (
	"context"
	"errors"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"gorm.io/gorm"
)

var errUserNotFound = apperror.NotFound("user not found")

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return UserRepository{db: db}
}

func (r UserRepository) Save(ctx context.Context, u entities.User) error {
	return r.db.WithContext(ctx).Save(fromUser(u)).Error
}

func (r UserRepository) FindByGitHubID(ctx context.Context, githubID int64) (entities.User, error) {
	return r.findOne(ctx, "github_id = ?", githubID)
}

func (r UserRepository) FindByID(ctx context.Context, id string) (entities.User, error) {
	return r.findOne(ctx, "id = ?", id)
}

func (r UserRepository) FindByLogin(ctx context.Context, login string) (entities.User, error) {
	return r.findOne(ctx, "login = ?", login)
}

func (r UserRepository) FindByIDs(ctx context.Context, ids []string) ([]entities.User, error) {
	return r.findMany(ctx, r.db.WithContext(ctx).Where("id IN ?", ids))
}

func (r UserRepository) List(ctx context.Context) ([]entities.User, error) {
	return r.findMany(ctx, r.db.WithContext(ctx))
}

func (r UserRepository) findOne(ctx context.Context, query string, arg any) (entities.User, error) {
	var model user
	err := r.db.WithContext(ctx).Where(query, arg).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.User{}, errUserNotFound
	}
	return model.toEntity(), err
}

func (r UserRepository) findMany(_ context.Context, tx *gorm.DB) ([]entities.User, error) {
	var models []user
	if err := tx.Order("login").Find(&models).Error; err != nil {
		return nil, err
	}
	users := make([]entities.User, 0, len(models))
	for _, m := range models {
		users = append(users, m.toEntity())
	}
	return users, nil
}
