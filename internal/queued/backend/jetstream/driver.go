package jetstream

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/adrianozp/platform/internal/queued/model"
	"github.com/nats-io/nats.go"
	js "github.com/nats-io/nats.go/jetstream"
)

const (
	dispatchConsumer = "dispatch"
	moverConsumer    = "mover"
	dedupeWindow     = 2 * time.Minute
)

type Driver struct {
	js        js.JetStream
	mu        sync.Mutex
	consumers map[string]js.Consumer
}

func New(conn *nats.Conn) (*Driver, error) {
	stream, err := js.New(conn)
	return &Driver{js: stream, consumers: map[string]js.Consumer{}}, err
}

func (d *Driver) Ensure(ctx context.Context, q model.Queue) error {
	for _, cfg := range streamConfigs(q) {
		if _, err := d.js.CreateOrUpdateStream(ctx, cfg); err != nil {
			return err
		}
	}
	if err := d.ensureConsumer(ctx, q.Stream(), dispatchConfig(q)); err != nil {
		return err
	}
	return d.ensureConsumer(ctx, q.DelayStream(), js.ConsumerConfig{
		Durable: moverConsumer, AckPolicy: js.AckExplicitPolicy, AckWait: 30 * time.Second, MaxDeliver: -1, MaxAckPending: -1,
	})
}

func (d *Driver) ensureConsumer(ctx context.Context, stream string, cfg js.ConsumerConfig) error {
	consumer, err := d.js.CreateOrUpdateConsumer(ctx, stream, cfg)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.consumers[stream+"/"+cfg.Durable] = consumer
	return nil
}

func (d *Driver) consumer(ctx context.Context, stream, name string) (js.Consumer, error) {
	d.mu.Lock()
	consumer, ok := d.consumers[stream+"/"+name]
	d.mu.Unlock()
	if ok {
		return consumer, nil
	}
	return d.js.Consumer(ctx, stream, name)
}

func streamConfigs(q model.Queue) []js.StreamConfig {
	size := int32(q.Settings.MaxMessageBytes + 8192)
	return []js.StreamConfig{
		{Name: q.Stream(), Subjects: []string{q.Subject()}, Retention: js.WorkQueuePolicy, MaxAge: q.Settings.Retention, MaxMsgSize: size, Duplicates: dedupeWindow, Storage: js.FileStorage},
		{Name: q.DelayStream(), Subjects: []string{q.DelaySubject()}, Retention: js.WorkQueuePolicy, MaxAge: q.Settings.Retention, MaxMsgSize: size, Duplicates: dedupeWindow, Storage: js.FileStorage},
		{Name: q.DeadStream(), Subjects: []string{q.DeadSubject()}, MaxAge: q.Settings.Retention, MaxMsgSize: size, Storage: js.FileStorage, AllowDirect: true},
	}
}

func dispatchConfig(q model.Queue) js.ConsumerConfig {
	return js.ConsumerConfig{
		Durable: dispatchConsumer, AckPolicy: js.AckExplicitPolicy, AckWait: q.Settings.DeliveryTimeout + 5*time.Second,
		MaxDeliver: -1, MaxAckPending: max(2*q.Settings.MaxInFlight, 10),
	}
}

func (d *Driver) Remove(ctx context.Context, q model.Queue) error {
	var errs []error
	for _, name := range []string{q.Stream(), q.DelayStream(), q.DeadStream()} {
		if err := d.js.DeleteStream(ctx, name); err != nil && !errors.Is(err, js.ErrStreamNotFound) {
			errs = append(errs, err)
		}
	}
	d.forget(q)
	return errors.Join(errs...)
}

func (d *Driver) forget(q model.Queue) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.consumers, q.Stream()+"/"+dispatchConsumer)
	delete(d.consumers, q.DelayStream()+"/"+moverConsumer)
}

func (d *Driver) Stats(ctx context.Context, q model.Queue) (model.BackendStats, error) {
	main, err := d.streamState(ctx, q.Stream())
	if err != nil {
		return model.BackendStats{}, err
	}
	delayed, delayErr := d.streamState(ctx, q.DelayStream())
	dead, deadErr := d.streamState(ctx, q.DeadStream())
	inFlight, flightErr := d.inFlight(ctx, q)
	stats := model.BackendStats{Depth: int64(main.Msgs + delayed.Msgs), InFlight: inFlight, DeadLetters: int64(dead.Msgs)}
	if main.Msgs > 0 {
		stats.OldestAge = time.Since(main.FirstTime)
	}
	return stats, errors.Join(delayErr, deadErr, flightErr)
}

func (d *Driver) streamState(ctx context.Context, name string) (js.StreamState, error) {
	stream, err := d.js.Stream(ctx, name)
	if err != nil {
		return js.StreamState{}, err
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return js.StreamState{}, err
	}
	return info.State, nil
}

func (d *Driver) inFlight(ctx context.Context, q model.Queue) (int, error) {
	consumer, err := d.consumer(ctx, q.Stream(), dispatchConsumer)
	if err != nil {
		return 0, err
	}
	info, err := consumer.Info(ctx)
	if err != nil {
		return 0, err
	}
	return info.NumAckPending, nil
}
