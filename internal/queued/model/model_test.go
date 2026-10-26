package model_test

import (
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
	"github.com/stretchr/testify/require"
)

func apiQueue() queuedapi.Queue {
	return queuedapi.Queue{
		ID: "q1", Name: "place-import", Environment: "prod", Version: 2, Target: "http://worker-prod:8081/hooks/place-import",
		Settings: queuedapi.Settings{
			Path: "/hooks/place-import", RatePerSecond: 5, MaxInFlight: 2, MaxAttempts: 3,
			Backoff:         queuedapi.Backoff{Kind: "exponential", Initial: "1s", Max: "5m0s"},
			DeliveryTimeout: "30s", Retention: "168h0m0s", MaxMessageBytes: 262144,
		},
		PublisherHashes: []string{"h1"}, SigningKey: "k", PreviousKey: "old",
	}
}

func Test_FromAPI_ParsesDurations(t *testing.T) {
	q, err := model.FromAPI(apiQueue())

	require.NoError(t, err)
	require.Equal(t, time.Second, q.Settings.Backoff.Initial)
	require.Equal(t, 5*time.Minute, q.Settings.Backoff.Max)
	require.Equal(t, 30*time.Second, q.Settings.DeliveryTimeout)
	require.Equal(t, 168*time.Hour, q.Settings.Retention)
	require.Equal(t, []string{"k", "old"}, q.Keys())
}

func Test_FromAPI_KeepsTheAdaptiveRate(t *testing.T) {
	adaptive := apiQueue()
	adaptive.Settings.Adaptive, adaptive.Settings.MinRatePerSecond = true, 0.5

	q, err := model.FromAPI(adaptive)

	require.NoError(t, err)
	require.True(t, q.Settings.Adaptive)
	require.Equal(t, 0.5, q.Settings.MinRatePerSecond)
}

func Test_FromAPI_RejectsBadDuration(t *testing.T) {
	bad := apiQueue()
	bad.Settings.Retention = "forever"

	_, err := model.FromAPI(bad)

	require.ErrorContains(t, err, "retention")
}

func Test_StreamNames(t *testing.T) {
	q, _ := model.FromAPI(apiQueue())

	require.Equal(t, "Q_PROD_PLACE_IMPORT", q.Stream())
	require.Equal(t, "DLQ_PROD_PLACE_IMPORT", q.DeadStream())
	require.Equal(t, "QD_PROD_PLACE_IMPORT", q.DelayStream())
	require.Equal(t, "q.prod.place-import", q.Subject())
	require.Equal(t, "dlq.prod.place-import", q.DeadSubject())
	require.Equal(t, "qd.prod.place-import", q.DelaySubject())
}
