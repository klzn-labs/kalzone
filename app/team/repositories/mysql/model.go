package mysql

import (
	"time"

	"github.com/adrianozp/platform/app/team/entities"
)

type team struct {
	ID        string `gorm:"primaryKey"`
	Slug      string
	Name      string
	CreatedAt time.Time
}

func (team) TableName() string { return "teams" }

func (t team) toEntity() entities.Team {
	return entities.Team{ID: t.ID, Slug: t.Slug, Name: t.Name, CreatedAt: t.CreatedAt}
}

type membership struct {
	TeamID    string `gorm:"primaryKey"`
	UserID    string `gorm:"primaryKey"`
	Role      string
	CreatedAt time.Time
}

func (membership) TableName() string { return "memberships" }

func (m membership) toEntity() entities.Membership {
	return entities.Membership{TeamID: m.TeamID, UserID: m.UserID, Role: entities.Role(m.Role), CreatedAt: m.CreatedAt}
}
