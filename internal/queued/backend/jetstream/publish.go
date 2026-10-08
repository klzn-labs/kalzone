package jetstream

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/adrianozp/platform/internal/queued/model"
	"github.com/nats-io/nats.go"
	js "github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nuid"
)

const (
	headerID          = "Platform-Message-Id"
	headerPublishedAt = "Platform-Published-At"
	headerDue         = "Platform-Due"
	headerContentType = "Content-Type"
	headerLastStatus  = "Platform-Last-Status"
	headerLastError   = "Platform-Last-Error"
	headerAttempts    = "Platform-Attempts"
)

func (d *Driver) Publish(ctx context.Context, q model.Queue, m model.Outgoing) (string, error) {
	id := messageID(m.IdempotencyKey)
	msg := nats.NewMsg(q.Subject())
	msg.Data = m.Body
	msg.Header.Set(headerID, id)
	msg.Header.Set(headerPublishedAt, time.Now().UTC().Format(time.RFC3339Nano))
	msg.Header.Set(headerContentType, m.ContentType)
	if m.Due.After(time.Now()) {
		msg.Subject = q.DelaySubject()
		msg.Header.Set(headerDue, strconv.FormatInt(m.Due.UnixMilli(), 10))
	}
	_, err := d.js.PublishMsg(ctx, msg, js.WithMsgID(id))
	return id, err
}

func messageID(idempotencyKey string) string {
	if idempotencyKey == "" {
		return nuid.Next()
	}
	sum := sha256.Sum256([]byte(idempotencyKey))
	return "k" + hex.EncodeToString(sum[:12])
}

func (d *Driver) MoveDue(ctx context.Context, q model.Queue, max int) (int, error) {
	consumer, err := d.consumer(ctx, q.DelayStream(), moverConsumer)
	if err != nil {
		return 0, err
	}
	batch, err := consumer.FetchNoWait(max)
	if err != nil {
		return 0, err
	}
	moved := 0
	for msg := range batch.Messages() {
		done, err := d.moveIfDue(ctx, q, msg)
		if err != nil {
			return moved, err
		}
		moved += done
	}
	return moved, batch.Error()
}

func (d *Driver) moveIfDue(ctx context.Context, q model.Queue, msg js.Msg) (int, error) {
	due, _ := strconv.ParseInt(msg.Headers().Get(headerDue), 10, 64)
	if wait := time.Until(time.UnixMilli(due)); wait > 0 {
		return 0, msg.NakWithDelay(wait)
	}
	out := nats.NewMsg(q.Subject())
	out.Data = msg.Data()
	for _, h := range []string{headerID, headerPublishedAt, headerContentType} {
		out.Header.Set(h, msg.Headers().Get(h))
	}
	if _, err := d.js.PublishMsg(ctx, out, js.WithMsgID(out.Header.Get(headerID))); err != nil {
		return 0, err
	}
	return 1, msg.DoubleAck(ctx)
}
