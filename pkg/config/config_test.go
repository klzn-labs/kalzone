package config_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/pkg/config"
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

func Test_LoadConfig_ReadsTheManagedDatabaseSettings(t *testing.T) {
	t.Setenv("PLATFORM_DATABASES_ADMIN_DSN", "admin:pw@tcp(mysql:3306)/")

	cfg, err := config.LoadConfig()

	require.NoError(t, err)
	require.Equal(t, "admin:pw@tcp(mysql:3306)/", cfg.Databases.AdminDSN)
	require.Equal(t, "mysql", cfg.Databases.Host)
	require.Equal(t, 3306, cfg.Databases.Port)
}

func Test_LoadConfig_PublishesMetricsToTheStackPushgateway(t *testing.T) {
	cfg, err := config.LoadConfig()

	require.NoError(t, err)
	require.Equal(t, "http://pushgateway:9091", cfg.Metrics.PushgatewayURL)
}

func Test_LoadConfig_ReadsTheDemoMember(t *testing.T) {
	t.Setenv("PLATFORM_DEMO_ENABLED", "true")
	t.Setenv("PLATFORM_DEMO_MEMBER", "adrianozp")

	cfg, err := config.LoadConfig()

	require.NoError(t, err)
	require.Equal(t, "adrianozp", cfg.Demo.Member)
}
