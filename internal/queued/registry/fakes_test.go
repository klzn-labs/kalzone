package registry_test

import (
	"context"
	"errors"
	"sync"
	"time"

	queuedbackend "github.com/adrianozp/platform/internal/queued/backend"
	"github.com/adrianozp/platform/internal/queued/model"
	"github.com/adrianozp/platform/internal/queued/registry"
	"github.com/adrianozp/platform/pkg/queuedapi"
)

var ctx = context.Background()

type backend struct {
	mu         sync.Mutex
	calls      []string
	replays    []string
	ensureFail bool
}

func (b *backend) log(call string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, call)
}

func (b *backend) Ensure(_ context.Context, q model.Queue) error {
	b.log("ensure " + q.Name)
	if b.ensureFail {
		return errors.New("nats unavailable")
	}
	return nil
}
func (b *backend) Remove(_ context.Context, q model.Queue) error {
	b.log("remove " + q.Environment + "/" + q.Name)
	return nil
}
func (b *backend) Publish(context.Context, model.Queue, model.Outgoing) (string, error) {
	return "", nil
}
func (b *backend) Fetch(context.Context, model.Queue, int, time.Duration) ([]model.Delivery, error) {
	return nil, nil
}
func (b *backend) MoveDue(context.Context, model.Queue, int) (int, error)     { return 0, nil }
func (b *backend) Ack(context.Context, model.Delivery) error                  { return nil }
func (b *backend) Retry(context.Context, model.Delivery, time.Duration) error { return nil }
func (b *backend) Bury(context.Context, model.Queue, model.Delivery, model.DeadReason) error {
	return nil
}
func (b *backend) Dead(context.Context, model.Queue, int) ([]model.DeadLetter, error) {
	return nil, nil
}
func (b *backend) Replay(_ context.Context, q model.Queue, id string) error {
	b.log("replay " + q.Name + " " + id)
	if id == "gone" {
		return queuedbackend.ErrDeadNotFound
	}
	return nil
}
func (b *backend) ReplayAll(_ context.Context, q model.Queue) (int, error) {
	b.log("replay_all " + q.Name)
	return 1, nil
}
func (b *backend) PurgeDead(_ context.Context, q model.Queue) error {
	b.log("purge " + q.Name)
	return nil
}
func (b *backend) Stats(context.Context, model.Queue) (model.BackendStats, error) {
	return model.BackendStats{}, nil
}

type dispatcher struct {
	started, stopped bool
	updates          []model.Queue
}

func (d *dispatcher) Start(context.Context) { d.started = true }
func (d *dispatcher) Update(q model.Queue)  { d.updates = append(d.updates, q) }
func (d *dispatcher) Stop()                 { d.stopped = true }

type factory struct{ made map[string]*dispatcher }

func (f *factory) make(q model.Queue) registry.Dispatcher {
	d := &dispatcher{}
	f.made[q.ID] = d
	return d
}

func newRegistry() (*registry.Registry, *backend, *factory) {
	b, f := &backend{}, &factory{made: map[string]*dispatcher{}}
	return registry.New(ctx, b, f.make), b, f
}

func apiQueue(id, name string, version int) queuedapi.Queue {
	return queuedapi.Queue{
		ID: id, Name: name, Environment: "dev", Version: version, Target: "http://c/hooks",
		Settings: queuedapi.Settings{
			Path: "/hooks", RatePerSecond: 5, MaxInFlight: 2, MaxAttempts: 3,
			Backoff:         queuedapi.Backoff{Kind: "fixed", Initial: "1s", Max: "1s"},
			DeliveryTimeout: "5s", Retention: "1h", MaxMessageBytes: 1024,
		},
		PublisherHashes: []string{"hash-" + name}, SigningKey: "key",
	}
}
