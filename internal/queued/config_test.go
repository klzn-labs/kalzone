package queued_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued"
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

func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("QUEUED_PLATFORM_URL", "https://platform.kalz.one")
	t.Setenv("QUEUED_TOKEN", "tok")
	t.Setenv("QUEUED_NATS_URL", "nats://nats:4222")
}

func Test_LoadConfig_LeavesTheGovernorOffByDefault(t *testing.T) {
	baseEnv(t)

	cfg, err := queued.LoadConfig()

	require.NoError(t, err)
	require.Empty(t, cfg.Adaptive.PrometheusURL)
	require.Equal(t, 50.0, cfg.Adaptive.Governor.Setpoint)
	require.Equal(t, 15*time.Second, cfg.Adaptive.Governor.Period)
	require.Equal(t, 3, cfg.Adaptive.Governor.StaleAfter)
	require.Contains(t, cfg.Adaptive.Query, `container_label_com_docker_compose_service="mysql"`)
}

func Test_LoadConfig_ReadsTheGovernorSettings(t *testing.T) {
	baseEnv(t)
	t.Setenv("QUEUED_ADAPTIVE_PROMETHEUS_URL", "http://prometheus:9090")
	t.Setenv("QUEUED_ADAPTIVE_SETPOINT", "60")
	t.Setenv("QUEUED_ADAPTIVE_KP", "0.02")
	t.Setenv("QUEUED_ADAPTIVE_PERIOD", "30s")

	cfg, err := queued.LoadConfig()

	require.NoError(t, err)
	require.Equal(t, "http://prometheus:9090", cfg.Adaptive.PrometheusURL)
	require.Equal(t, 60.0, cfg.Adaptive.Governor.Setpoint)
	require.Equal(t, 0.02, cfg.Adaptive.Governor.Kp)
	require.Equal(t, 30*time.Second, cfg.Adaptive.Governor.Period)
}

func Test_LoadConfig_RefusesABrokenGovernor(t *testing.T) {
	baseEnv(t)
	t.Setenv("QUEUED_ADAPTIVE_STALE_AFTER", "0")

	_, err := queued.LoadConfig()

	require.ErrorContains(t, err, "STALE_AFTER")
}
