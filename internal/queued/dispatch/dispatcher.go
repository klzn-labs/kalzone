package dispatch

import (
	"context"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/backend"
	"github.com/klzn-labs/kalzone/internal/queued/model"
	"golang.org/x/time/rate"
)

const (
	pausedWait = 250 * time.Millisecond
	fullWait   = 20 * time.Millisecond
	fetchWait  = 500 * time.Millisecond
	moveBatch  = 100
)

type Recorder interface {
	Delivered(queueID string, verdict Verdict, took time.Duration)
}

type Dispatcher struct {
	backend  backend.Backend
	sender   sender
	recorder Recorder
	mu       sync.Mutex
	queue    model.Queue
	limiter  *rate.Limiter
	inFlight atomic.Int64
	cancel   context.CancelFunc
	loop     sync.WaitGroup
	work     sync.WaitGroup
}

func New(b backend.Backend, client httpDoer, q model.Queue, recorder Recorder) *Dispatcher {
	d := &Dispatcher{backend: b, sender: sender{client: client}, recorder: recorder, queue: q}
	d.limiter = rate.NewLimiter(rate.Limit(q.Settings.RatePerSecond), 1)
	return d
}

func (d *Dispatcher) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	d.cancel = cancel
	d.loop.Add(1)
	go d.run(ctx)
}

func (d *Dispatcher) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
	d.loop.Wait()
	d.work.Wait()
}

func (d *Dispatcher) Update(q model.Queue) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queue = q
	d.limiter.SetLimit(rate.Limit(q.Settings.RatePerSecond))
}

func (d *Dispatcher) snapshot() model.Queue {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.queue
}

func (d *Dispatcher) run(ctx context.Context) {
	defer d.loop.Done()
	for ctx.Err() == nil {
		d.step(ctx, d.snapshot())
	}
}

func (d *Dispatcher) step(ctx context.Context, q model.Queue) {
	if q.Paused {
		sleep(ctx, pausedWait)
		return
	}
	d.backend.MoveDue(ctx, q, moveBatch)
	if int(d.inFlight.Load()) >= q.Settings.MaxInFlight {
		sleep(ctx, fullWait)
		return
	}
	if err := d.limiter.Wait(ctx); err != nil {
		return
	}
	batch, err := d.backend.Fetch(ctx, q, 1, fetchWait)
	if err != nil {
		sleep(ctx, fetchWait)
	}
	for _, m := range batch {
		d.launch(ctx, q, m)
	}
}

func (d *Dispatcher) launch(ctx context.Context, q model.Queue, m model.Delivery) {
	d.inFlight.Add(1)
	d.work.Add(1)
	go func() {
		defer d.work.Done()
		defer d.inFlight.Add(-1)
		d.deliver(ctx, q, m)
	}()
}

func (d *Dispatcher) deliver(ctx context.Context, q model.Queue, m model.Delivery) {
	started := time.Now()
	result := d.sender.send(ctx, q, m)
	if ctx.Err() != nil {
		return
	}
	verdict, after := Outcome(result.status, result.err, result.header)
	if verdict == Retry && m.Attempt >= q.Settings.MaxAttempts {
		verdict = Bury
	}
	d.settle(ctx, q, m, verdict, after, result)
	d.recorder.Delivered(q.ID, verdict, time.Since(started))
}

func (d *Dispatcher) settle(ctx context.Context, q model.Queue, m model.Delivery, verdict Verdict, after time.Duration, result sendResult) {
	switch verdict {
	case Ack:
		d.backend.Ack(ctx, m)
	case Retry:
		d.backend.Retry(ctx, m, Delay(q.Settings.Backoff, m.Attempt, after, rand.Float64))
	case Bury:
		d.backend.Bury(ctx, q, m, model.DeadReason{Status: result.status, Error: result.describe(), Attempts: m.Attempt})
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
