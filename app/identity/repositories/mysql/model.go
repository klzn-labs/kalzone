package mysql

import (
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
)

type user struct {
	ID           string `gorm:"primaryKey"`
	GitHubID     *int64 `gorm:"column:github_id"`
	Kind         string
	Login        string
	Name         string
	AvatarURL    string
	IsAdmin      bool
	CreatedAt    time.Time
	LastLoginAt  time.Time
	PasswordHash *string
	DisabledAt   *time.Time
}

func (user) TableName() string { return "users" }

func (u user) toEntity() entities.User {
	return entities.User{
		ID: u.ID, GitHubID: valueOf(u.GitHubID), Kind: entities.Kind(u.Kind), Login: u.Login, Name: u.Name,
		AvatarURL: u.AvatarURL, Admin: u.IsAdmin, CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt,
		PasswordHash: stringValue(u.PasswordHash), DisabledAt: u.DisabledAt,
	}
}

func valueOf(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}

func pointerOf(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}

func kindOf(k entities.Kind) string {
	if k == "" {
		return string(entities.KindPerson)
	}
	return string(k)
}

func fromUser(u entities.User) user {
	return user{
		ID: u.ID, GitHubID: pointerOf(u.GitHubID), Kind: kindOf(u.Kind), Login: u.Login, Name: u.Name,
		AvatarURL: u.AvatarURL, IsAdmin: u.Admin, CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt,
		PasswordHash: stringPointer(u.PasswordHash), DisabledAt: u.DisabledAt,
	}
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func stringPointer(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type session struct {
	TokenHash string `gorm:"primaryKey"`
	CSRFToken string `gorm:"column:csrf_token"`
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (session) TableName() string { return "sessions" }

func (s session) toEntity() entities.Session {
	return entities.Session{TokenHash: s.TokenHash, CSRFToken: s.CSRFToken, UserID: s.UserID, CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt}
}

func fromSession(s entities.Session) session {
	return session{TokenHash: s.TokenHash, CSRFToken: s.CSRFToken, UserID: s.UserID, CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt}
}
