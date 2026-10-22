package mysql

import (
	"context"
	"errors"

	"github.com/go-sql-driver/mysql"
	"github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"gorm.io/gorm"
)

const duplicateEntry = 1062

var (
	errTeamNotFound = apperror.NotFound("team not found").WithHint("list teams with GET /api/v1/teams")
	errTeamTaken    = apperror.Conflict("a team with this slug already exists")
)

type TeamRepository struct {
	db *gorm.DB
}

func NewTeamRepository(db *gorm.DB) TeamRepository {
	return TeamRepository{db: db}
}

func (r TeamRepository) Create(ctx context.Context, t entities.Team) error {
	err := r.db.WithContext(ctx).Create(&team{ID: t.ID, Slug: t.Slug, Name: t.Name, IsDemo: t.Demo, CreatedAt: t.CreatedAt}).Error
	if isDuplicate(err) {
		return errTeamTaken
	}
	return err
}

func (r TeamRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("team_id = ?", id).Delete(&membership{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&team{}).Error
	})
}

func (r TeamRepository) FindBySlug(ctx context.Context, slug string) (entities.Team, error) {
	return r.findOne(ctx, "slug = ?", slug)
}

func (r TeamRepository) FindByID(ctx context.Context, id string) (entities.Team, error) {
	return r.findOne(ctx, "id = ?", id)
}

func (r TeamRepository) List(ctx context.Context) ([]entities.Team, error) {
	return r.findMany(r.db.WithContext(ctx))
}

func (r TeamRepository) FindByIDs(ctx context.Context, ids []string) ([]entities.Team, error) {
	return r.findMany(r.db.WithContext(ctx).Where("id IN ?", ids))
}

func (r TeamRepository) DemoTeamIDs(ctx context.Context) ([]string, error) {
	ids := []string{}
	err := r.db.WithContext(ctx).Model(&team{}).Where("is_demo").Order("id").Pluck("id", &ids).Error
	return ids, err
}

func (r TeamRepository) findOne(ctx context.Context, query string, arg any) (entities.Team, error) {
	var model team
	err := r.db.WithContext(ctx).Where(query, arg).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Team{}, errTeamNotFound
	}
	return model.toEntity(), err
}

func (r TeamRepository) findMany(tx *gorm.DB) ([]entities.Team, error) {
	var models []team
	if err := tx.Order("slug").Find(&models).Error; err != nil {
		return nil, err
	}
	teams := make([]entities.Team, 0, len(models))
	for _, m := range models {
		teams = append(teams, m.toEntity())
	}
	return teams, nil
}

func isDuplicate(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == duplicateEntry
}
