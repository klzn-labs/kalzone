package mysql_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/testdb"
	"github.com/stretchr/testify/require"
)

func controlplane(version string, at time.Time) entities.PlatformComponent {
	return entities.PlatformComponent{
		Name: "controlplane", Agent: "vps-1", ObservedAt: at,
		Live:      entities.PlatformRelease{Version: version, Image: "img:" + version, Colour: "green", UpdatedAt: at.Add(-time.Hour)},
		Container: entities.PlatformContainer{Found: true, Name: "controlplane-green", State: "running", Health: "healthy"},
		History: []entities.PlatformDeploy{
			{At: at.Add(-time.Hour), Version: version, Outcome: entities.OutcomeDeployed, Strategy: "bluegreen", Colour: "green"},
			{At: at.Add(-2 * time.Hour), Version: version, Outcome: entities.OutcomeRolledBack, Step: "gate", Reason: "p95 too high"},
		},
	}
}

func Test_SavePlatformKeepsTheLatestReport(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))
	at := time.Now().UTC().Truncate(time.Millisecond)

	require.NoError(t, repo.SavePlatform(ctx, controlplane("abc", at)))
	require.NoError(t, repo.SavePlatform(ctx, controlplane("def", at.Add(time.Minute))))
	got, found, err := repo.Platform(ctx, "controlplane")

	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "def", got.Live.Version)
	require.Equal(t, at.Add(time.Minute), got.ObservedAt)
	require.Len(t, got.History, 2)
	require.Equal(t, "p95 too high", got.History[1].Reason)
	require.Equal(t, "controlplane-green", got.Container.Name)
}

func Test_AnUnreportedPlatformComponentIsNotFound(t *testing.T) {
	repo := mysql.NewDeploymentRepository(testdb.Open(t))

	_, found, err := repo.Platform(ctx, "controlplane")

	require.NoError(t, err)
	require.False(t, found)
}
