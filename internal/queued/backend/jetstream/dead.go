package jetstream

import (
	"context"
	"strconv"
	"time"

	"github.com/adrianozp/platform/internal/queued/backend"
	"github.com/adrianozp/platform/internal/queued/model"
	"github.com/nats-io/nats.go"
	js "github.com/nats-io/nats.go/jetstream"
)

func (d *Driver) Dead(ctx context.Context, q model.Queue, limit int) ([]model.DeadLetter, error) {
	stream, info, err := d.deadStream(ctx, q)
	if err != nil || info.Msgs == 0 {
		return []model.DeadLetter{}, err
	}
	result := []model.DeadLetter{}
	for seq := info.LastSeq; seq >= info.FirstSeq && len(result) < limit; seq-- {
		raw, err := stream.GetMsg(ctx, seq)
		if err == nil {
			result = append(result, toDead(raw))
		}
	}
	return result, nil
}

func (d *Driver) deadStream(ctx context.Context, q model.Queue) (js.Stream, js.StreamState, error) {
	stream, err := d.js.Stream(ctx, q.DeadStream())
	if err != nil {
		return nil, js.StreamState{}, err
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return nil, js.StreamState{}, err
	}
	return stream, info.State, nil
}

func toDead(raw *js.RawStreamMsg) model.DeadLetter {
	status, _ := strconv.Atoi(raw.Header.Get(headerLastStatus))
	attempts, _ := strconv.Atoi(raw.Header.Get(headerAttempts))
	published, _ := time.Parse(time.RFC3339Nano, raw.Header.Get(headerPublishedAt))
	return model.DeadLetter{
		ID: raw.Header.Get(headerID), Attempts: attempts, LastStatus: status,
		LastError: raw.Header.Get(headerLastError), PublishedAt: published, Body: raw.Data,
	}
}

func (d *Driver) Replay(ctx context.Context, q model.Queue, id string) error {
	moved, err := d.replayWhere(ctx, q, func(raw *js.RawStreamMsg) bool { return raw.Header.Get(headerID) == id })
	if err == nil && moved == 0 {
		return backend.ErrDeadNotFound
	}
	return err
}

func (d *Driver) ReplayAll(ctx context.Context, q model.Queue) (int, error) {
	return d.replayWhere(ctx, q, func(*js.RawStreamMsg) bool { return true })
}

func (d *Driver) replayWhere(ctx context.Context, q model.Queue, match func(*js.RawStreamMsg) bool) (int, error) {
	stream, info, err := d.deadStream(ctx, q)
	if err != nil || info.Msgs == 0 {
		return 0, err
	}
	moved := 0
	for seq := info.FirstSeq; seq <= info.LastSeq; seq++ {
		raw, err := stream.GetMsg(ctx, seq)
		if err != nil || !match(raw) {
			continue
		}
		if err := d.requeue(ctx, q, stream, raw); err != nil {
			return moved, err
		}
		moved++
	}
	return moved, nil
}

func (d *Driver) requeue(ctx context.Context, q model.Queue, stream js.Stream, raw *js.RawStreamMsg) error {
	out := nats.NewMsg(q.Subject())
	out.Data = raw.Data
	for _, h := range []string{headerID, headerPublishedAt, headerContentType} {
		out.Header.Set(h, raw.Header.Get(h))
	}
	if _, err := d.js.PublishMsg(ctx, out); err != nil {
		return err
	}
	return stream.DeleteMsg(ctx, raw.Sequence)
}

func (d *Driver) PurgeDead(ctx context.Context, q model.Queue) error {
	stream, err := d.js.Stream(ctx, q.DeadStream())
	if err != nil {
		return err
	}
	return stream.Purge(ctx)
}
