package entities_test

import (
	"strings"
	"testing"
	"time"

	"github.com/adrianozp/platform/app/identity/entities"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/stretchr/testify/require"
)

var day = 24 * time.Hour

func draft() entities.TokenDraft {
	return entities.TokenDraft{Name: "claude-local", Scopes: []string{"read"}, Teams: []string{"places"}}
}

func Test_TokenDraft_Refusals(t *testing.T) {
	cases := map[string]func(*entities.TokenDraft){
		"empty name":      func(d *entities.TokenDraft) { d.Name = "" },
		"long name":       func(d *entities.TokenDraft) { d.Name = strings.Repeat("a", 65) },
		"spaces":          func(d *entities.TokenDraft) { d.Name = "my token" },
		"no scopes":       func(d *entities.TokenDraft) { d.Scopes = nil },
		"unknown scope":   func(d *entities.TokenDraft) { d.Scopes = []string{"root"} },
		"no teams":        func(d *entities.TokenDraft) { d.Teams = nil },
		"negative expiry": func(d *entities.TokenDraft) { d.ExpiresInDays = -1 },
		"over a year":     func(d *entities.TokenDraft) { d.ExpiresInDays = 366 },
	}
	for name, change := range cases {
		d := draft()
		change(&d)
		_, err := d.Check()
		require.ErrorIs(t, err, apperror.Invalid(""), name)
	}
}

func Test_TokenDraft_ExpiryDefaultsTo90Days(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	d := draft()
	year := draft()
	year.ExpiresInDays = 365

	require.Equal(t, now.Add(90*day), d.ExpiresAt(now))
	require.Equal(t, now.Add(365*day), year.ExpiresAt(now))
}

func Test_AccessToken_Check(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	live := entities.AccessToken{ExpiresAt: now.Add(time.Second)}
	expired := entities.AccessToken{ExpiresAt: now}
	revoked := entities.AccessToken{ExpiresAt: now.Add(day), RevokedAt: &now}

	require.NoError(t, live.Check(now))
	require.ErrorIs(t, expired.Check(now), entities.ErrTokenExpired)
	require.ErrorIs(t, revoked.Check(now), entities.ErrTokenRevoked)
}
