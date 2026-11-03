package entities

import (
	"time"

	"github.com/klzn-labs/kalzone/pkg/uuid"
)

type LinkKind string

const (
	LinkSetup  LinkKind = "setup"
	LinkInvite LinkKind = "invite"
	LinkReset  LinkKind = "reset"
)

var linkLifetimes = map[LinkKind]time.Duration{
	LinkSetup:  24 * time.Hour,
	LinkInvite: 7 * 24 * time.Hour,
	LinkReset:  24 * time.Hour,
}

type AccountLink struct {
	ID             string
	Kind           LinkKind
	TokenHash      string
	TeamID         string
	Role           string
	UserID         string
	CreatedBy      string
	CreatedByLogin string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	UsedAt         *time.Time
	RevokedAt      *time.Time
}

func (l AccountLink) Live(now time.Time) bool {
	return l.UsedAt == nil && l.RevokedAt == nil && now.Before(l.ExpiresAt)
}

func NewAccountLink(kind LinkKind, now time.Time) (AccountLink, string, error) {
	token, err := NewToken()
	if err != nil {
		return AccountLink{}, "", err
	}
	link := AccountLink{ID: uuid.NewUUID(), Kind: kind, TokenHash: HashToken(token), CreatedAt: now, ExpiresAt: now.Add(linkLifetimes[kind])}
	return link, token, nil
}
