package entities_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/stretchr/testify/require"
)

func Test_PlatformRelease_SplitsCommitAndConfig(t *testing.T) {
	release := entities.PlatformRelease{Version: "c63f05e+cfg.4f2232a4"}

	require.Equal(t, "c63f05e", release.Commit())
	require.Equal(t, "cfg.4f2232a4", release.Config())
	require.Equal(t, "abc", entities.PlatformRelease{Version: "abc"}.Commit())
}

func Test_PlatformComponent_HealthNeedsARunningContainerThatIsNotUnhealthy(t *testing.T) {
	running := entities.PlatformComponent{Container: entities.PlatformContainer{Found: true, State: "running", Health: "healthy"}}
	unhealthy := entities.PlatformComponent{Container: entities.PlatformContainer{Found: true, State: "running", Health: "unhealthy"}}

	require.True(t, running.Healthy())
	require.False(t, unhealthy.Healthy())
	require.False(t, entities.PlatformComponent{}.Healthy())
}

func Test_PlatformComponent_LastRollbackIsTheNewestOne(t *testing.T) {
	c := entities.PlatformComponent{History: []entities.PlatformDeploy{
		{Version: "c", Outcome: entities.OutcomeDeployed},
		{Version: "b", Outcome: entities.OutcomeRolledBack},
		{Version: "a", Outcome: entities.OutcomeRolledBack},
	}}

	last, found := c.LastRollback()

	require.True(t, found)
	require.Equal(t, "b", last.Version)
	_, none := entities.PlatformComponent{}.LastRollback()
	require.False(t, none)
}

func Test_PlatformComponent_IsStaleWhenTheAgentStopsReporting(t *testing.T) {
	now := time.Now()
	c := entities.PlatformComponent{ObservedAt: now.Add(-entities.StaleAfter - time.Second)}

	require.True(t, c.Stale(now))
	require.False(t, entities.PlatformComponent{ObservedAt: now}.Stale(now))
}
