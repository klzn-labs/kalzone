package jetstream_test

import (
	"context"
	"testing"
	"time"

	"github.com/adrianozp/platform/internal/queued/backend/jetstream"
	"github.com/adrianozp/platform/internal/queued/model"
	"github.com/adrianozp/platform/internal/queued/natstest"
	nativejs "github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

func setup(t *testing.T, name string) (*jetstream.Driver, model.Queue) {
	t.Helper()
	d, err := jetstream.New(natstest.Start(t))
	require.NoError(t, err)
	q := natstest.Queue(name)
	require.NoError(t, d.Ensure(ctx, q))
	return d, q
}

func fetchOne(t *testing.T, d *jetstream.Driver, q model.Queue) model.Delivery {
	t.Helper()
	got, err := d.Fetch(ctx, q, 1, 2*time.Second)
	require.NoError(t, err)
	require.Len(t, got, 1)
	return got[0]
}

func Test_Publish_FetchAck(t *testing.T) {
	d, q := setup(t, "jobs")

	id, err := d.Publish(ctx, q, model.Outgoing{Body: []byte(`{"a":1}`), ContentType: "application/json"})
	got := fetchOne(t, d, q)
	ackErr := d.Ack(ctx, got)

	require.NoError(t, err)
	require.NoError(t, ackErr)
	require.Equal(t, id, got.ID)
	require.Equal(t, `{"a":1}`, string(got.Body))
	require.Equal(t, "application/json", got.ContentType)
	require.Equal(t, 1, got.Attempt)
	require.False(t, got.PublishedAt.IsZero())
	stats, _ := d.Stats(ctx, q)
	require.EqualValues(t, 0, stats.Depth)
}

func Test_Publish_DeduplicatesTheIdempotencyKey(t *testing.T) {
	d, q := setup(t, "jobs")

	first, _ := d.Publish(ctx, q, model.Outgoing{Body: []byte("1"), IdempotencyKey: "city-7"})
	second, err := d.Publish(ctx, q, model.Outgoing{Body: []byte("2"), IdempotencyKey: "city-7"})

	require.NoError(t, err)
	require.Equal(t, first, second)
	stats, _ := d.Stats(ctx, q)
	require.EqualValues(t, 1, stats.Depth)
}

func Test_Retry_RedeliversAfterTheDelayWithTheNextAttempt(t *testing.T) {
	d, q := setup(t, "jobs")
	d.Publish(ctx, q, model.Outgoing{Body: []byte("x")})
	first := fetchOne(t, d, q)

	require.NoError(t, d.Retry(ctx, first, 300*time.Millisecond))
	early, _ := d.Fetch(ctx, q, 1, 100*time.Millisecond)
	again := fetchOne(t, d, q)

	require.Empty(t, early)
	require.Equal(t, first.ID, again.ID)
	require.Equal(t, 2, again.Attempt)
}

func Test_Bury_MovesToDeadLettersWithTheReason(t *testing.T) {
	d, q := setup(t, "jobs")
	d.Publish(ctx, q, model.Outgoing{Body: []byte("x")})
	got := fetchOne(t, d, q)

	require.NoError(t, d.Bury(ctx, q, got, model.DeadReason{Status: 422, Error: "bad payload", Attempts: 1}))

	dead, err := d.Dead(ctx, q, 10)
	require.NoError(t, err)
	require.Len(t, dead, 1)
	require.Equal(t, got.ID, dead[0].ID)
	require.Equal(t, 422, dead[0].LastStatus)
	require.Equal(t, "bad payload", dead[0].LastError)
	require.Equal(t, "x", string(dead[0].Body))
	stats, _ := d.Stats(ctx, q)
	require.EqualValues(t, 0, stats.Depth)
	require.EqualValues(t, 1, stats.DeadLetters)
}

func Test_ReplayAndReplayAll_ReturnDeadLettersToTheQueue(t *testing.T) {
	d, q := setup(t, "jobs")
	for _, body := range []string{"a", "b", "c"} {
		d.Publish(ctx, q, model.Outgoing{Body: []byte(body)})
		d.Bury(ctx, q, fetchOne(t, d, q), model.DeadReason{Status: 500, Attempts: 3})
	}
	dead, _ := d.Dead(ctx, q, 10)

	require.NoError(t, d.Replay(ctx, q, dead[0].ID))
	moved, err := d.ReplayAll(ctx, q)

	require.NoError(t, err)
	require.Equal(t, 2, moved)
	stats, _ := d.Stats(ctx, q)
	require.EqualValues(t, 3, stats.Depth)
	require.EqualValues(t, 0, stats.DeadLetters)
	require.Equal(t, 1, fetchOne(t, d, q).Attempt)
}

func Test_PurgeDead(t *testing.T) {
	d, q := setup(t, "jobs")
	d.Publish(ctx, q, model.Outgoing{Body: []byte("x")})
	d.Bury(ctx, q, fetchOne(t, d, q), model.DeadReason{Status: 500})

	require.NoError(t, d.PurgeDead(ctx, q))

	stats, _ := d.Stats(ctx, q)
	require.EqualValues(t, 0, stats.DeadLetters)
}

func Test_DelayedPublish_IsMovedWhenDue(t *testing.T) {
	d, q := setup(t, "jobs")
	id, _ := d.Publish(ctx, q, model.Outgoing{Body: []byte("later"), Due: time.Now().Add(400 * time.Millisecond)})

	movedEarly, _ := d.MoveDue(ctx, q, 10)
	early, _ := d.Fetch(ctx, q, 1, 100*time.Millisecond)
	time.Sleep(500 * time.Millisecond)
	movedLate, err := d.MoveDue(ctx, q, 10)
	got := fetchOne(t, d, q)

	require.NoError(t, err)
	require.Equal(t, 0, movedEarly)
	require.Empty(t, early)
	require.Equal(t, 1, movedLate)
	require.Equal(t, id, got.ID)
	require.Equal(t, 1, got.Attempt)
}

func Test_Stats_CountsDepthInFlightAndDelayed(t *testing.T) {
	d, q := setup(t, "jobs")
	d.Publish(ctx, q, model.Outgoing{Body: []byte("1")})
	d.Publish(ctx, q, model.Outgoing{Body: []byte("2")})
	d.Publish(ctx, q, model.Outgoing{Body: []byte("3"), Due: time.Now().Add(time.Hour)})
	fetchOne(t, d, q)

	stats, err := d.Stats(ctx, q)

	require.NoError(t, err)
	require.EqualValues(t, 3, stats.Depth)
	require.Equal(t, 1, stats.InFlight)
	require.Greater(t, stats.OldestAge, time.Duration(0))
}

func Test_Unacked_IsRedeliveredAfterAckWait(t *testing.T) {
	d, q := setup(t, "jobs")
	d.Publish(ctx, q, model.Outgoing{Body: []byte("x")})
	fetchOne(t, d, q)

	again, err := d.Fetch(ctx, q, 1, 8*time.Second)

	require.NoError(t, err)
	require.Len(t, again, 1)
	require.Equal(t, 2, again[0].Attempt)
}

func Test_Ensure_UpdatesSettingsAndKeepsMessages(t *testing.T) {
	d, q := setup(t, "jobs")
	d.Publish(ctx, q, model.Outgoing{Body: []byte("x")})
	q.Settings.Retention = 2 * time.Hour
	q.Settings.MaxInFlight = 8

	require.NoError(t, d.Ensure(ctx, q))

	stats, _ := d.Stats(ctx, q)
	require.EqualValues(t, 1, stats.Depth)
}

func Test_Remove_DeletesTheStreams(t *testing.T) {
	d, q := setup(t, "jobs")

	require.NoError(t, d.Remove(ctx, q))

	_, err := d.Stats(ctx, q)
	require.Error(t, err)
}

func Test_Ensure_TheMoverIsNotCappedByMessagesNotYetDue(t *testing.T) {
	conn := natstest.Start(t)
	d, err := jetstream.New(conn)
	require.NoError(t, err)
	q := natstest.Queue("jobs")
	require.NoError(t, d.Ensure(ctx, q))
	stream, err := nativejs.New(conn)
	require.NoError(t, err)

	mover, err := stream.Consumer(ctx, q.DelayStream(), "mover")
	require.NoError(t, err)
	info, err := mover.Info(ctx)

	require.NoError(t, err)
	require.Equal(t, -1, info.Config.MaxAckPending)
}
