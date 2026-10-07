package entities

import "time"

type User struct {
	ID          string
	GitHubID    int64
	Login       string
	Name        string
	AvatarURL   string
	Admin       bool
	CreatedAt   time.Time
	LastLoginAt time.Time
}

type Session struct {
	TokenHash string
	CSRFToken string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type GitHubUser struct {
	ID        int64
	Login     string
	Name      string
	AvatarURL string
}

type IssuedSession struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}
