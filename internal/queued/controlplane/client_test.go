package controlplane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/klzn-labs/kalzone/internal/queued/controlplane"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
	"github.com/stretchr/testify/require"
)

type fakePlatform struct {
	reported queuedapi.StatsReport
	runs     queuedapi.CronRunReport
	done     string
}

func (f *fakePlatform) serve(t *testing.T) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case queuedapi.DesiredPath:
			json.NewEncoder(w).Encode(queuedapi.Desired{Queues: []queuedapi.Queue{{ID: "q1"}}})
		case queuedapi.StatsPath:
			json.NewDecoder(r.Body).Decode(&f.reported)
		case queuedapi.CronRunsPath:
			json.NewDecoder(r.Body).Decode(&f.runs)
		case queuedapi.DonePath("c1"):
			f.done = "c1"
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server
}

func Test_Client_DesiredReportAndDone(t *testing.T) {
	platform := &fakePlatform{}
	server := platform.serve(t)
	c := controlplane.New(server.URL, "tok", server.Client())

	desired, err := c.Desired(context.Background())
	reportErr := c.Report(context.Background(), []queuedapi.Stats{{QueueID: "q1", Depth: 3}})
	doneErr := c.Done(context.Background(), "c1")

	require.NoError(t, err)
	require.NoError(t, reportErr)
	require.NoError(t, doneErr)
	require.Equal(t, "q1", desired.Queues[0].ID)
	require.EqualValues(t, 3, platform.reported.Stats[0].Depth)
	require.Equal(t, "c1", platform.done)
}

func Test_Client_ReportsAnUnreachableOrFailingControlPlane(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer failing.Close()

	_, unreachable := controlplane.New("http://127.0.0.1:1", "tok", http.DefaultClient).Desired(context.Background())
	_, status := controlplane.New(failing.URL, "tok", failing.Client()).Desired(context.Background())

	require.Error(t, unreachable)
	require.ErrorContains(t, status, "503")
}

func Test_Client_ReportsCronRuns(t *testing.T) {
	platform := &fakePlatform{}
	server := platform.serve(t)

	err := controlplane.New(server.URL, "tok", server.Client()).ReportRuns(context.Background(), []queuedapi.CronRun{{ID: "r1", Status: "succeeded"}})

	require.NoError(t, err)
	require.Equal(t, []queuedapi.CronRun{{ID: "r1", Status: "succeeded"}}, platform.runs.Runs)
}
