package stats_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/internal/queued/dispatch"
	"github.com/adrianozp/platform/internal/queued/model"
	"github.com/adrianozp/platform/internal/queued/natstest"
	"github.com/adrianozp/platform/internal/queued/stats"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

type source struct{ queues []model.Queue }

func (s source) Queues() []model.Queue { return s.queues }

type backend struct{}

func (backend) Stats(context.Context, model.Queue) (model.BackendStats, error) {
	return model.BackendStats{Depth: 4, InFlight: 1, OldestAge: 3 * time.Second, DeadLetters: 1}, nil
}

func (backend) Dead(context.Context, model.Queue, int) ([]model.DeadLetter, error) {
	return []model.DeadLetter{{ID: "m9", Attempts: 5, LastStatus: 500, Body: make([]byte, 5000)}}, nil
}

func Test_Collector_ReportsBackendStatsRatesAndDeadPreviews(t *testing.T) {
	q := natstest.Queue("jobs")
	now := time.Now()
	c := stats.New(source{queues: []model.Queue{q}}, backend{}, prometheus.NewRegistry(), func() time.Time { return now })
	c.Delivered(q.ID, dispatch.Ack, time.Millisecond)
	c.Delivered(q.ID, dispatch.Ack, time.Millisecond)
	c.Delivered(q.ID, dispatch.Retry, time.Millisecond)

	got := c.Collect(context.Background())

	require.Len(t, got, 1)
	require.Equal(t, q.ID, got[0].QueueID)
	require.EqualValues(t, 4, got[0].Depth)
	require.Equal(t, 1, got[0].InFlight)
	require.InDelta(t, 3, got[0].OldestAgeSeconds, 0.01)
	require.InDelta(t, 2, got[0].DeliveredPerMinute, 0.01)
	require.InDelta(t, 1, got[0].FailedPerMinute, 0.01)
	require.Len(t, got[0].NewestDead, 1)
	require.Len(t, got[0].NewestDead[0].BodyPreview, 4096)
}

func Test_Collector_ForgetsDeliveriesOlderThanAMinute(t *testing.T) {
	q := natstest.Queue("jobs")
	now := time.Now()
	c := stats.New(source{queues: []model.Queue{q}}, backend{}, prometheus.NewRegistry(), func() time.Time { return now })
	c.Delivered(q.ID, dispatch.Ack, time.Millisecond)
	now = now.Add(61 * time.Second)

	got := c.Collect(context.Background())

	require.Zero(t, got[0].DeliveredPerMinute)
}
