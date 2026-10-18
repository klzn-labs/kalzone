package mysql

import (
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
)

type user struct {
	ID          string `gorm:"primaryKey"`
	GitHubID    int64  `gorm:"column:github_id"`
	Login       string
	Name        string
	AvatarURL   string
	IsAdmin     bool
	CreatedAt   time.Time
	LastLoginAt time.Time
}

func (user) TableName() string { return "users" }

func (u user) toEntity() entities.User {
	return entities.User{
		ID: u.ID, GitHubID: u.GitHubID, Login: u.Login, Name: u.Name, AvatarURL: u.AvatarURL,
		Admin: u.IsAdmin, CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt,
	}
}

func fromUser(u entities.User) user {
	return user{
		ID: u.ID, GitHubID: u.GitHubID, Login: u.Login, Name: u.Name, AvatarURL: u.AvatarURL,
		IsAdmin: u.Admin, CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt,
	}
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
