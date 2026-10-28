package mysql

import (
	"context"

	"gorm.io/gorm"
)

const personas = "DELETE FROM users WHERE kind = 'persona'"

type WipeRepository struct {
	db *gorm.DB
}

func NewWipeRepository(db *gorm.DB) WipeRepository {
	return WipeRepository{db: db}
}

func (r WipeRepository) Wipe(ctx context.Context, teamID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := deleteTeamRows(tx, teamID); err != nil {
			return err
		}
		return tx.Exec(personas).Error
	})
}

func deleteTeamRows(tx *gorm.DB, teamID string) error {
	for i := len(shifted) - 1; i >= 0; i-- {
		if err := tx.Exec("DELETE FROM "+shifted[i].table+" WHERE "+shifted[i].where, teamID).Error; err != nil {
			return err
		}
	}
	return nil
}
