package jetstream

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/nats-io/nats.go"
	js "github.com/nats-io/nats.go/jetstream"
)

var errHandle = errors.New("delivery has no JetStream message")

func (d *Driver) Fetch(ctx context.Context, q model.Queue, max int, wait time.Duration) ([]model.Delivery, error) {
	consumer, err := d.consumer(ctx, q.Stream(), dispatchConsumer)
	if err != nil {
		return nil, err
	}
	batch, err := consumer.Fetch(max, js.FetchMaxWait(wait))
	if err != nil {
		return nil, err
	}
	result := []model.Delivery{}
	for msg := range batch.Messages() {
		result = append(result, toDelivery(q, msg))
	}
	if err := batch.Error(); err != nil && !errors.Is(err, nats.ErrTimeout) {
		return result, err
	}
	return result, nil
}

func toDelivery(q model.Queue, msg js.Msg) model.Delivery {
	attempt := 1
	if meta, err := msg.Metadata(); err == nil {
		attempt = int(meta.NumDelivered)
	}
	published, _ := time.Parse(time.RFC3339Nano, msg.Headers().Get(headerPublishedAt))
	return model.Delivery{
		ID: msg.Headers().Get(headerID), QueueID: q.ID, Body: msg.Data(), ContentType: msg.Headers().Get(headerContentType),
		PublishedAt: published, Attempt: attempt, Handle: msg,
	}
}

func message(d model.Delivery) (js.Msg, error) {
	msg, ok := d.Handle.(js.Msg)
	if !ok {
		return nil, errHandle
	}
	return msg, nil
}

func (d *Driver) Ack(ctx context.Context, delivery model.Delivery) error {
	msg, err := message(delivery)
	if err != nil {
		return err
	}
	return msg.DoubleAck(ctx)
}

func (d *Driver) Retry(_ context.Context, delivery model.Delivery, after time.Duration) error {
	msg, err := message(delivery)
	if err != nil {
		return err
	}
	return msg.NakWithDelay(after)
}

func (d *Driver) Bury(ctx context.Context, q model.Queue, delivery model.Delivery, reason model.DeadReason) error {
	msg, err := message(delivery)
	if err != nil {
		return err
	}
	dead := nats.NewMsg(q.DeadSubject())
	dead.Data = msg.Data()
	for _, h := range []string{headerID, headerPublishedAt, headerContentType} {
		dead.Header.Set(h, msg.Headers().Get(h))
	}
	dead.Header.Set(headerLastStatus, strconv.Itoa(reason.Status))
	dead.Header.Set(headerLastError, reason.Error)
	dead.Header.Set(headerAttempts, strconv.Itoa(reason.Attempts))
	if _, err := d.js.PublishMsg(ctx, dead); err != nil {
		return err
	}
	return msg.DoubleAck(ctx)
}
