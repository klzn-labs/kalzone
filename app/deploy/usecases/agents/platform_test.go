package agents_test

import (
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_ReportPlatform_StoresTheControlPlaneStampedByTheAgent(t *testing.T) {
	store, _, usecase := setup(false)
	at := time.Now().UTC()

	err := usecase.ReportPlatform(ctx, "vps-1", at, entities.PlatformComponent{Name: "controlplane", Live: entities.PlatformRelease{Version: "abc"}})

	require.NoError(t, err)
	stored := store.Platforms["controlplane"]
	require.Equal(t, "abc", stored.Live.Version)
	require.Equal(t, "vps-1", stored.Agent)
	require.Equal(t, at, stored.ObservedAt)
}

func Test_ReportPlatform_RefusesUnknownComponents(t *testing.T) {
	store, _, usecase := setup(false)

	err := usecase.ReportPlatform(ctx, "vps-1", time.Now(), entities.PlatformComponent{Name: "api-prod"})

	require.Equal(t, apperror.CodeNotFound, apperror.CodeOf(err))
	require.Empty(t, store.Platforms)
}

func Test_ReportPlatform_ClipsHistoryAndReasons(t *testing.T) {
	store, _, usecase := setup(false)
	history := make([]entities.PlatformDeploy, 30)
	history[0].Reason = strings.Repeat("é", 900)

	require.NoError(t, usecase.ReportPlatform(ctx, "vps-1", time.Now(), entities.PlatformComponent{Name: "controlplane", History: history}))

	stored := store.Platforms["controlplane"].History
	require.Len(t, stored, 20)
	require.Len(t, []rune(stored[0].Reason), 500)
}
