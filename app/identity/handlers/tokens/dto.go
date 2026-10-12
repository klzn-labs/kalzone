package tokens

import (
	"time"

	"github.com/adrianozp/platform/app/identity/entities"
	"github.com/adrianozp/platform/pkg/scope"
)

type createDTO struct {
	Name          string   `json:"name" binding:"required"`
	Scopes        []string `json:"scopes" binding:"required"`
	Teams         []string `json:"teams" binding:"required"`
	Bypass        bool     `json:"bypass"`
	ExpiresInDays int      `json:"expires_in_days"`
}

func (d createDTO) toEntity() entities.TokenDraft {
	return entities.TokenDraft{Name: d.Name, Scopes: d.Scopes, Teams: d.Teams, Bypass: d.Bypass, ExpiresInDays: d.ExpiresInDays}
}

type TokenDTO struct {
	ID         string        `json:"id"`
	OwnerID    string        `json:"owner_id"`
	Name       string        `json:"name"`
	Prefix     string        `json:"prefix"`
	Scopes     []scope.Scope `json:"scopes"`
	TeamIDs    []string      `json:"team_ids"`
	Bypass     bool          `json:"bypass"`
	CreatedAt  time.Time     `json:"created_at"`
	ExpiresAt  time.Time     `json:"expires_at"`
	LastUsedAt *time.Time    `json:"last_used_at,omitempty"`
	LastUsedIP string        `json:"last_used_ip,omitempty"`
	RevokedAt  *time.Time    `json:"revoked_at,omitempty"`
}

type issuedDTO struct {
	TokenDTO
	Token string `json:"token"`
}

func ToDTO(t entities.AccessToken) TokenDTO {
	return TokenDTO{
		ID: t.ID, OwnerID: t.OwnerID, Name: t.Name, Prefix: t.Prefix, Scopes: t.Scopes, TeamIDs: t.TeamIDs,
		Bypass: t.Bypass, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt,
		LastUsedIP: t.LastUsedIP, RevokedAt: t.RevokedAt,
	}
}

type ownerDTO struct {
	ID    string `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	Admin bool   `json:"admin"`
}

type tokenInfoDTO struct {
	Name      string        `json:"name"`
	Prefix    string        `json:"prefix"`
	Scopes    []scope.Scope `json:"scopes"`
	Teams     []string      `json:"teams"`
	Bypass    bool          `json:"bypass"`
	ExpiresAt time.Time     `json:"expires_at"`
}

type whoamiDTO struct {
	Owner ownerDTO      `json:"owner"`
	Token *tokenInfoDTO `json:"token"`
}

func toWhoami(w entities.Whoami) whoamiDTO {
	result := whoamiDTO{Owner: ownerDTO{ID: w.User.ID, Login: w.User.Login, Name: w.User.Name, Admin: w.User.Admin}}
	if w.Token.ID == "" {
		return result
	}
	result.Token = &tokenInfoDTO{
		Name: w.Token.Name, Prefix: w.Token.Prefix, Scopes: w.Token.Scopes, Teams: w.Teams,
		Bypass: w.Token.Bypass, ExpiresAt: w.Token.ExpiresAt,
	}
	return result
}
