package entities

import "time"

type Kind string

const (
	KindPerson  Kind = "person"
	KindGuest   Kind = "guest"
	KindPersona Kind = "persona"
)

type User struct {
	ID          string
	GitHubID    int64
	Kind        Kind
	Login       string
	Name        string
	AvatarURL   string
	Admin       bool
	CreatedAt   time.Time
	LastLoginAt time.Time
}

func (u User) Guest() bool {
	return u.Kind == KindGuest
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
