package mysql

import (
	"encoding/json"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
)

type accessToken struct {
	ID               string `gorm:"primaryKey"`
	OwnerID          string
	Name             string
	TokenHash        string
	DisplayPrefix    string
	Scopes           []byte
	TeamIDs          []byte `gorm:"column:team_ids"`
	Bypass           bool
	CreatedAt        time.Time
	CreatedByTokenID string `gorm:"column:created_by_token_id"`
	ExpiresAt        time.Time
	LastUsedAt       *time.Time
	LastUsedIP       string `gorm:"column:last_used_ip"`
	RevokedAt        *time.Time
}

func (accessToken) TableName() string { return "access_tokens" }

func fromAccessToken(t entities.AccessToken) (accessToken, error) {
	scopes, err := json.Marshal(t.Scopes)
	if err != nil {
		return accessToken{}, err
	}
	teams, err := json.Marshal(t.TeamIDs)
	return accessToken{
		ID: t.ID, OwnerID: t.OwnerID, Name: t.Name, TokenHash: t.Hash, DisplayPrefix: t.Prefix,
		Scopes: scopes, TeamIDs: teams, Bypass: t.Bypass, CreatedAt: t.CreatedAt,
		CreatedByTokenID: t.CreatedByTokenID, ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt,
		LastUsedIP: t.LastUsedIP, RevokedAt: t.RevokedAt,
	}, err
}

func (m accessToken) toEntity() (entities.AccessToken, error) {
	t := entities.AccessToken{
		ID: m.ID, OwnerID: m.OwnerID, Name: m.Name, Hash: m.TokenHash, Prefix: m.DisplayPrefix,
		Bypass: m.Bypass, CreatedAt: m.CreatedAt.UTC(), CreatedByTokenID: m.CreatedByTokenID,
		ExpiresAt: m.ExpiresAt.UTC(), LastUsedAt: m.LastUsedAt, LastUsedIP: m.LastUsedIP, RevokedAt: m.RevokedAt,
	}
	if err := json.Unmarshal(m.Scopes, &t.Scopes); err != nil {
		return entities.AccessToken{}, err
	}
	return t, json.Unmarshal(m.TeamIDs, &t.TeamIDs)
}
