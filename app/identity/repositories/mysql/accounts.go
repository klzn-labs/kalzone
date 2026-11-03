package mysql

import (
	"context"
	"slices"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const claimLink = "UPDATE account_links SET used_at = ? WHERE id = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > ?"

var (
	errUsernameTaken = apperror.Conflict("this username is taken")
	errHasOwner      = apperror.Conflict("this Kalzone already has an owner")
	errLastAdmin     = apperror.Conflict("keep at least one admin")
)

type AccountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) AccountRepository {
	return AccountRepository{db: db}
}

func (r AccountRepository) CompleteSetup(ctx context.Context, linkID string, owner entities.User, team entities.NewTeam) error {
	return r.claimed(ctx, linkID, func(tx *gorm.DB) error {
		if err := requireNoPeople(tx); err != nil {
			return err
		}
		if err := tx.Create(ptr(fromUser(owner))).Error; err != nil {
			return mapUsernameTaken(err)
		}
		row := map[string]any{"id": team.ID, "slug": team.Slug, "name": team.Name, "created_at": team.CreatedAt}
		if err := tx.Table("teams").Create(row).Error; err != nil {
			return mapOwnerExists(err)
		}
		return addMembership(tx, team.ID, owner, "maintainer")
	})
}

func requireNoPeople(tx *gorm.DB) error {
	n, err := countPeople(tx)
	if err != nil {
		return err
	}
	if n > 0 {
		return errHasOwner
	}
	return nil
}

func mapOwnerExists(err error) error {
	if isDuplicate(err) {
		return errHasOwner
	}
	return err
}

func countPeople(tx *gorm.DB) (int, error) {
	var n int64
	err := tx.Model(&user{}).Where("kind = ?", string(entities.KindPerson)).Count(&n).Error
	return int(n), err
}

func (r AccountRepository) AcceptInvite(ctx context.Context, link entities.AccountLink, user entities.User) error {
	err := r.claimed(ctx, link.ID, func(tx *gorm.DB) error {
		if err := tx.Create(ptr(fromUser(user))).Error; err != nil {
			return err
		}
		if err := requireTeam(tx, link.TeamID); err != nil {
			return err
		}
		return addMembership(tx, link.TeamID, user, link.Role)
	})
	return mapUsernameTaken(err)
}

func (r AccountRepository) ResetPassword(ctx context.Context, link entities.AccountLink, hash string) error {
	return r.claimed(ctx, link.ID, func(tx *gorm.DB) error {
		err := tx.Model(&user{}).Where("id = ?", link.UserID).Update("password_hash", hash).Error
		if err != nil {
			return err
		}
		return tx.Where("user_id = ?", link.UserID).Delete(&session{}).Error
	})
}

func requireTeam(tx *gorm.DB, teamID string) error {
	var n int64
	err := tx.Table("teams").Clauses(clause.Locking{Strength: "SHARE"}).Where("id = ?", teamID).Count(&n).Error
	if err != nil {
		return err
	}
	if n == 0 {
		return errLinkGone
	}
	return nil
}

func (r AccountRepository) ChangePassword(ctx context.Context, userID, hash, keepSessionHash string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user{}).Where("id = ?", userID).Update("password_hash", hash).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ? AND token_hash <> ?", userID, keepSessionHash).Delete(&session{}).Error
	})
}

func (r AccountRepository) Disable(ctx context.Context, userID string, at time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := revokeLinksOf(tx, userID, at); err != nil {
			return err
		}
		if err := lockForRemoval(tx, userID); err != nil {
			return err
		}
		for _, step := range disableSteps(userID, at) {
			if err := step(tx); err != nil {
				return err
			}
		}
		return nil
	})
}

func lockForRemoval(tx *gorm.DB, userID string) error {
	wasAdmin, err := isActiveAdmin(tx, userID)
	if err != nil {
		return err
	}
	if wasAdmin {
		return keepAnotherAdmin(tx, userID)
	}
	isAdmin, err := lockAndCheckAdmin(tx, userID)
	if err != nil || !isAdmin {
		return err
	}
	return keepAnotherAdmin(tx, userID)
}

func isActiveAdmin(tx *gorm.DB, userID string) (bool, error) {
	var n int64
	err := activeAdmins(tx.Model(&user{})).Where("id = ?", userID).Count(&n).Error
	return n > 0, err
}

func lockAndCheckAdmin(tx *gorm.DB, userID string) (bool, error) {
	var rows []user
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return false, err
	}
	return rows[0].IsAdmin && rows[0].DisabledAt == nil, nil
}

func keepAnotherAdmin(tx *gorm.DB, userID string) error {
	var ids []string
	if err := activeAdmins(tx.Model(&user{})).Pluck("id", &ids).Error; err != nil {
		return err
	}
	admins, err := lockStillActiveAdmins(tx, append(ids, userID))
	if err != nil || !slices.Contains(admins, userID) {
		return err
	}
	if len(admins) == 1 {
		return errLastAdmin
	}
	return nil
}

func lockStillActiveAdmins(tx *gorm.DB, ids []string) ([]string, error) {
	var rows []user
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Order("id").Find(&rows).Error
	var admins []string
	for _, row := range rows {
		if row.IsAdmin && row.DisabledAt == nil {
			admins = append(admins, row.ID)
		}
	}
	return admins, err
}

func activeAdmins(tx *gorm.DB) *gorm.DB {
	return tx.Where("is_admin = TRUE AND disabled_at IS NULL")
}

func disableSteps(userID string, at time.Time) []func(*gorm.DB) error {
	return []func(*gorm.DB) error{
		func(tx *gorm.DB) error { return tx.Exec("DELETE FROM memberships WHERE user_id = ?", userID).Error },
		func(tx *gorm.DB) error { return tx.Where("user_id = ?", userID).Delete(&session{}).Error },
		func(tx *gorm.DB) error {
			return tx.Model(&accessToken{}).Where("owner_id = ? AND revoked_at IS NULL", userID).Update("revoked_at", at).Error
		},
		func(tx *gorm.DB) error {
			return tx.Model(&user{}).Where("id = ?", userID).Updates(map[string]any{"is_admin": false, "disabled_at": at}).Error
		},
	}
}

func revokeLinksOf(tx *gorm.DB, userID string, at time.Time) error {
	err := unusedLinks(tx).Where("user_id = ?", userID).Update("revoked_at", at).Error
	if err != nil {
		return err
	}
	return unusedLinks(tx).Where("created_by = ? AND kind = ?", userID, string(entities.LinkInvite)).Update("revoked_at", at).Error
}

func unusedLinks(tx *gorm.DB) *gorm.DB {
	return tx.Model(&accountLink{}).Where("used_at IS NULL AND revoked_at IS NULL")
}

func mapUsernameTaken(err error) error {
	if isDuplicate(err) {
		return errUsernameTaken
	}
	return err
}

func (r AccountRepository) CountPeople(ctx context.Context) (int, error) {
	return countPeople(r.db.WithContext(ctx))
}

func (r AccountRepository) claimed(ctx context.Context, linkID string, apply func(*gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := clock.Now().UTC()
		res := tx.Exec(claimLink, now, linkID, now)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errLinkGone
		}
		return apply(tx)
	})
}

func addMembership(tx *gorm.DB, teamID string, u entities.User, role string) error {
	row := map[string]any{"team_id": teamID, "user_id": u.ID, "role": role, "created_at": u.CreatedAt}
	return tx.Table("memberships").Create(row).Error
}
