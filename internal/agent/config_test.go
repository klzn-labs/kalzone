package agent_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/internal/agent"
	"github.com/stretchr/testify/require"
)

func Test_LoadConfig_ReadsInternalEntrypointsPerEnvironment(t *testing.T) {
	t.Setenv("AGENT_PLATFORM_URL", "https://platform")
	t.Setenv("AGENT_TOKEN", "token")
	t.Setenv("AGENT_INTERNAL_ENTRYPOINTS", "prod=backend, dev=backend-dev")

	cfg, err := agent.LoadConfig("v1")

	require.NoError(t, err)
	require.Equal(t, map[string]string{"prod": "backend", "dev": "backend-dev"}, cfg.InternalEntrypoints)
}

func Test_LoadConfig_RejectsAMalformedEntrypointPair(t *testing.T) {
	t.Setenv("AGENT_PLATFORM_URL", "https://platform")
	t.Setenv("AGENT_TOKEN", "token")
	t.Setenv("AGENT_INTERNAL_ENTRYPOINTS", "prod")

	_, err := agent.LoadConfig("v1")

	require.ErrorContains(t, err, "AGENT_INTERNAL_ENTRYPOINTS")
}
