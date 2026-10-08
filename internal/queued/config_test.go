package queued_test

import (
	"testing"
	"time"

	"github.com/adrianozp/platform/internal/queued"
	"github.com/stretchr/testify/require"
)

func Test_LoadConfig_ReadsTheEnvironmentWithDefaults(t *testing.T) {
	t.Setenv("QUEUED_PLATFORM_URL", "https://platform.kalz.one")
	t.Setenv("QUEUED_TOKEN", "tok")
	t.Setenv("QUEUED_NATS_URL", "nats://nats:4222")
	t.Setenv("QUEUED_POLL", "2s")

	cfg, err := queued.LoadConfig()

	require.NoError(t, err)
	require.Equal(t, "https://platform.kalz.one", cfg.PlatformURL)
	require.Equal(t, 2*time.Second, cfg.Poll)
	require.Equal(t, 15*time.Second, cfg.StatsEvery)
	require.Equal(t, ":8080", cfg.Addr)
	require.Equal(t, ":9090", cfg.MetricsAddr)
}

func Test_LoadConfig_NeedsThePlatformTokenAndNATS(t *testing.T) {
	t.Setenv("QUEUED_PLATFORM_URL", "https://platform.kalz.one")

	_, err := queued.LoadConfig()

	require.ErrorContains(t, err, "QUEUED_TOKEN")
}
