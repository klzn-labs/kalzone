package mysql

import (
	"context"
	"errors"

	"github.com/klzn-labs/kalzone/app/team/entities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r TeamRepository) PutMembership(ctx context.Context, m entities.Membership) error {
	model := membership{TeamID: m.TeamID, UserID: m.UserID, Role: string(m.Role), CreatedAt: m.CreatedAt}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		DoUpdates: clause.AssignmentColumns([]string{"role"}),
	}).Create(&model).Error
}

func (r TeamRepository) DeleteMembership(ctx context.Context, teamID, userID string) error {
	return r.db.WithContext(ctx).Where("team_id = ? AND user_id = ?", teamID, userID).Delete(&membership{}).Error
}

func (r TeamRepository) Membership(ctx context.Context, teamID, userID string) (entities.Membership, bool, error) {
	var model membership
	err := r.db.WithContext(ctx).Where("team_id = ? AND user_id = ?", teamID, userID).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.Membership{}, false, nil
	}
	return model.toEntity(), err == nil, err
}

func (r TeamRepository) MembershipsOfTeam(ctx context.Context, teamID string) ([]entities.Membership, error) {
	return r.memberships(r.db.WithContext(ctx).Where("team_id = ?", teamID))
}

func (r TeamRepository) MembershipsOfUser(ctx context.Context, userID string) ([]entities.Membership, error) {
	return r.memberships(r.db.WithContext(ctx).Where("user_id = ?", userID))
}

func (r TeamRepository) CountMaintainers(ctx context.Context, teamID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&membership{}).
		Where("team_id = ? AND role = ?", teamID, entities.RoleMaintainer).Count(&count).Error
	return count, err
}

func (r TeamRepository) memberships(tx *gorm.DB) ([]entities.Membership, error) {
	var models []membership
	if err := tx.Order("created_at").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.Membership, 0, len(models))
	for _, m := range models {
		result = append(result, m.toEntity())
	}
	return result, nil
}
