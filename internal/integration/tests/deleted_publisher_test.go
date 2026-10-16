package tests

import (
	"net/http"
	"testing"

	"github.com/adrianozp/platform/internal/integration"
	"github.com/adrianozp/platform/pkg/queuedapi"
	"github.com/stretchr/testify/require"
)

func Test_ADeletedPublishersQueueTokenStopsWorking(t *testing.T) {
	s, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "dev", "release": builtRelease(t, s, p)}, http.StatusAccepted, nil)
	_, claimed := claim(t, s)
	require.Equal(t, http.StatusOK, claimed)
	publisherHashes(t, s)

	beforeDelete := publisherHashes(t, s)
	deleteService(t, p.mia, "places-api")
	afterDelete := publisherHashes(t, s)

	require.Len(t, beforeDelete, 2)
	require.Len(t, afterDelete, 1)
	require.Contains(t, beforeDelete, afterDelete[0])
}

func publisherHashes(t *testing.T, s *integration.Suite) []string {
	t.Helper()
	var desired queuedapi.Desired
	decode(t, s.Queued(t, http.MethodGet, "/queued/v1/desired", nil).Body, &desired)
	require.Len(t, desired.Queues, 1)
	return desired.Queues[0].PublisherHashes
}
