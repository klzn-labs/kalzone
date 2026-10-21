package deployctl_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeState(t *testing.T, root, file, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "state"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "state", file), []byte(body), 0o644))
}

func Test_Platform_ReadsTheLiveReleaseHistoryAndContainer(t *testing.T) {
	runtime, _ := setup(t)
	writeState(t, runtime.Root, "api-dev.json", `{"colour":"blue","image":"img:abc","version":"abc+cfg.1","updated_at":"2026-10-21T16:19:17Z"}`)
	writeState(t, runtime.Root, "api-dev.history.json", `[{"at":"2026-10-21T16:09:31Z","version":"abc+cfg.1","outcome":"rolled_back","step":"gate","reason":"p95 latency 331ms"}]`)

	report, found, err := runtime.Platform(context.Background(), "api-dev")

	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "abc+cfg.1", report.Live.Version)
	require.Equal(t, "rolled_back", report.History[0].Outcome)
	require.Equal(t, "p95 latency 331ms", report.History[0].Reason)
	require.Equal(t, "api-dev-blue", report.Container.Name)
}

func Test_Platform_WithoutHistoryStillReportsTheLiveRelease(t *testing.T) {
	runtime, _ := setup(t)
	writeState(t, runtime.Root, "api-dev.json", `{"colour":"blue","version":"abc"}`)

	report, found, err := runtime.Platform(context.Background(), "api-dev")

	require.NoError(t, err)
	require.True(t, found)
	require.Empty(t, report.History)
}

func Test_Platform_NotDeployedOnThisHostIsNotFound(t *testing.T) {
	runtime, _ := setup(t)

	_, found, err := runtime.Platform(context.Background(), "controlplane")

	require.NoError(t, err)
	require.False(t, found)
}
