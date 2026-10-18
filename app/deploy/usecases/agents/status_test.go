package agents_test

import (
	"strings"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/stretchr/testify/require"
)

func Test_Finish_KeepsTheFailureClipped(t *testing.T) {
	store, _, usecase := setup(false)
	d := pending(store, "d1", "dev", entities.StatusPending)
	d.Status, d.Agent = entities.StatusInProgress, "vps-1"
	store.ByID["d1"] = d
	logs := make([]string, 70)
	for i := range logs {
		logs[i] = strings.Repeat("l", 3000)
	}

	_, err := usecase.Finish(ctx, "vps-1", "d1", false, "deployctl: exit status 10", &entities.Failure{Class: entities.FailureUnhealthy, Logs: logs})

	require.NoError(t, err)
	stored := store.ByID["d1"].Failure
	require.Equal(t, entities.FailureUnhealthy, stored.Class)
	require.Len(t, stored.Logs, 50)
	require.Len(t, stored.Logs[0], 2000)
}

func Test_Finish_DropsAFailureOnSuccess(t *testing.T) {
	store, _, usecase := setup(false)
	d := pending(store, "d1", "dev", entities.StatusPending)
	d.Status, d.Agent = entities.StatusInProgress, "vps-1"
	store.ByID["d1"] = d

	_, err := usecase.Finish(ctx, "vps-1", "d1", true, "live", &entities.Failure{Class: entities.FailureCrashed})

	require.NoError(t, err)
	require.Nil(t, store.ByID["d1"].Failure)
}

func Test_ReportStatus_StoresKnownWorkloadsAndSkipsUnknownOnes(t *testing.T) {
	store, _, usecase := setup(false)
	at := time.Now().UTC()
	snapshots := []entities.Snapshot{
		{Service: "api", Environment: "dev", Status: entities.RuntimeStatus{Container: "api-dev-blue", Found: true, State: "running"}},
		{Service: "gone", Environment: "dev", Status: entities.RuntimeStatus{Found: false}},
	}

	require.NoError(t, usecase.ReportStatus(ctx, "vps-1", at, snapshots))

	stored := store.Runtimes["s1/dev"]
	require.Len(t, store.Runtimes, 1)
	require.Equal(t, "running", stored.State)
	require.Equal(t, "vps-1", stored.Agent)
	require.Equal(t, "s1", stored.ServiceID)
	require.Equal(t, at, stored.ObservedAt)
}
