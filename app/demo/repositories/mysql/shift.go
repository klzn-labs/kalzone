package mysql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const teamServices = "SELECT id FROM services WHERE owner_team_id = ?"

var shifted = []struct {
	table   string
	columns []string
	where   string
}{
	{"teams", []string{"created_at"}, "id = ?"},
	{"memberships", []string{"created_at"}, "team_id = ?"},
	{"services", []string{"created_at"}, "owner_team_id = ?"},
	{"config_revisions", []string{"created_at"}, "service_id IN (" + teamServices + ")"},
	{"releases", []string{"created_at", "finished_at"}, "service_id IN (" + teamServices + ")"},
	{"release_changelogs", []string{"updated_at"}, "release_id IN (SELECT id FROM releases WHERE service_id IN (" + teamServices + "))"},
	{"deployments", []string{"created_at", "started_at", "finished_at"}, "service_id IN (" + teamServices + ")"},
	{"change_requests", []string{"created_at", "expires_at", "decided_at", "finished_at"}, "team_id = ?"},
	{"audit_events", []string{"at"}, "team_id = ?"},
}

type ShiftRepository struct {
	db *gorm.DB
}

func NewShiftRepository(db *gorm.DB) ShiftRepository {
	return ShiftRepository{db: db}
}

func (r ShiftRepository) Shift(ctx context.Context, teamID string, by time.Duration) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, s := range shifted {
			if err := tx.Exec(statement(s.table, s.columns, s.where, by), teamID).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func statement(table string, columns []string, where string, by time.Duration) string {
	sets := make([]string, 0, len(columns))
	for _, c := range columns {
		sets = append(sets, fmt.Sprintf("%s = %s + INTERVAL %d MICROSECOND", c, c, by.Microseconds()))
	}
	return "UPDATE " + table + " SET " + strings.Join(sets, ", ") + " WHERE " + where
}
