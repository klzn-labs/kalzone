package entities_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/stretchr/testify/require"
)

func Test_NewAccountLinkExpiry(t *testing.T) {
	now := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	cases := map[entities.LinkKind]time.Duration{
		entities.LinkSetup:  24 * time.Hour,
		entities.LinkReset:  24 * time.Hour,
		entities.LinkInvite: 168 * time.Hour,
	}
	for kind, ttl := range cases {
		link, token, err := entities.NewAccountLink(kind, now)
		require.NoError(t, err)
		require.Equal(t, now.Add(ttl), link.ExpiresAt, kind)
		require.Equal(t, entities.HashToken(token), link.TokenHash)
		require.NotEmpty(t, link.ID)
		require.Equal(t, now, link.CreatedAt)
	}
}

func Test_LiveLink(t *testing.T) {
	now := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	link, _, err := entities.NewAccountLink(entities.LinkInvite, now)
	require.NoError(t, err)
	require.True(t, link.Live(now))
	used, revoked := link, link
	used.UsedAt = &now
	revoked.RevokedAt = &now
	require.False(t, used.Live(now))
	require.False(t, revoked.Live(now))
	require.False(t, link.Live(link.ExpiresAt))
}
