package config_test

import (
	"testing"
	"time"

	"github.com/adrianozp/platform/pkg/config"
	"github.com/stretchr/testify/require"
)

func Test_LoadConfig_EnvironmentOverridesDefaults(t *testing.T) {
	t.Setenv("PLATFORM_BASE_URL", "https://platform.kalz.one")
	t.Setenv("PLATFORM_DB_DSN", "u:p@tcp(db:3306)/platform")
	t.Setenv("PLATFORM_ADMINS", "adrianozp,someone")
	t.Setenv("PLATFORM_ALLOWED_DOMAINS", "kalz.one")
	t.Setenv("PLATFORM_SESSION_TTL", "2h")

	cfg, err := config.LoadConfig()

	require.NoError(t, err)
	require.Equal(t, "https://platform.kalz.one", cfg.BaseURL)
	require.Equal(t, "u:p@tcp(db:3306)/platform", cfg.DB.DSN)
	require.Equal(t, []string{"adrianozp", "someone"}, cfg.Admins)
	require.Equal(t, []string{"kalz.one"}, cfg.AllowedDomains)
	require.Equal(t, 2*time.Hour, cfg.Session.TTL)
	require.True(t, cfg.SecureCookies())
}

func Test_LoadConfig_DefaultsWork(t *testing.T) {
	cfg, err := config.LoadConfig()

	require.NoError(t, err)
	require.Equal(t, ":8080", cfg.HTTP.Addr)
	require.Equal(t, "https://github.com", cfg.GitHub.OAuthURL)
	require.False(t, cfg.SecureCookies())
}

func Test_LoadConfig_ReadsTheQueueGatewaySettings(t *testing.T) {
	t.Setenv("PLATFORM_QUEUES_TOKEN", "queued-token")

	cfg, err := config.LoadConfig()

	require.NoError(t, err)
	require.Equal(t, "queued-token", cfg.Queues.Token)
	require.Equal(t, "http://queued:8080", cfg.Queues.GatewayURL)
}
