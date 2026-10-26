package stats

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/dispatch"
	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/klzn-labs/kalzone/pkg/queuedapi"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	window       = time.Minute
	deadReported = 50
	previewBytes = 4096
)

type QueueSource interface {
	Queues() []model.Queue
}

type Backend interface {
	Stats(ctx context.Context, q model.Queue) (model.BackendStats, error)
	Dead(ctx context.Context, q model.Queue, limit int) ([]model.DeadLetter, error)
}

type Throttle interface {
	Adaptive() model.AdaptiveState
}

type event struct {
	at        time.Time
	delivered bool
}

type Collector struct {
	queues   QueueSource
	backend  Backend
	throttle Throttle
	now      func() time.Time
	metrics  metrics
	mu       sync.Mutex
	events   map[string][]event
}

func New(queues QueueSource, backend Backend, throttle Throttle, registerer prometheus.Registerer, now func() time.Time) *Collector {
	return &Collector{queues: queues, backend: backend, throttle: throttle, now: now, metrics: newMetrics(registerer), events: map[string][]event{}}
}

func (c *Collector) Delivered(queueID string, verdict dispatch.Verdict, took time.Duration) {
	c.mu.Lock()
	c.events[queueID] = append(c.events[queueID], event{at: c.now(), delivered: verdict == dispatch.Ack})
	c.mu.Unlock()
	c.metrics.delivered(c.labels(queueID), verdict, took)
}

func (c *Collector) labels(queueID string) prometheus.Labels {
	for _, q := range c.queues.Queues() {
		if q.ID == queueID {
			return prometheus.Labels{"queue": q.Name, "environment": q.Environment}
		}
	}
	return prometheus.Labels{"queue": queueID, "environment": ""}
}

func (c *Collector) Collect(ctx context.Context) []queuedapi.Stats {
	result := []queuedapi.Stats{}
	for _, q := range c.queues.Queues() {
		s, err := c.collect(ctx, q)
		if err != nil {
			log.Printf("stats for queue %s/%s: %v", q.Environment, q.Name, err)
			continue
		}
		result = append(result, s)
	}
	return result
}

func (c *Collector) collect(ctx context.Context, q model.Queue) (queuedapi.Stats, error) {
	b, err := c.backend.Stats(ctx, q)
	if err != nil {
		return queuedapi.Stats{}, err
	}
	dead, err := c.backend.Dead(ctx, q, deadReported)
	delivered, failed := c.rates(q.ID)
	labels := prometheus.Labels{"queue": q.Name, "environment": q.Environment}
	c.metrics.observe(labels, b)
	s := queuedapi.Stats{
		QueueID: q.ID, Depth: b.Depth, InFlight: b.InFlight, OldestAgeSeconds: b.OldestAge.Seconds(),
		DeliveredPerMinute: delivered, FailedPerMinute: failed, DeadLetters: b.DeadLetters, NewestDead: previews(dead),
	}
	c.addRate(&s, q, labels)
	return s, err
}

func (c *Collector) addRate(s *queuedapi.Stats, q model.Queue, labels prometheus.Labels) {
	state := c.throttle.Adaptive()
	s.EffectiveRate = q.Settings.EffectiveRate(state.Factor)
	c.metrics.effective.With(labels).Set(s.EffectiveRate)
	if q.Settings.Adaptive && state.On {
		s.Adaptive = &queuedapi.Adaptive{Factor: state.Factor, DatabaseCPU: state.Measured, TargetCPU: state.Setpoint, FailSafe: state.FailSafe}
	}
}

func (c *Collector) rates(queueID string) (float64, float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := c.now().Add(-window)
	kept := []event{}
	delivered, failed := 0.0, 0.0
	for _, e := range c.events[queueID] {
		if e.at.Before(cutoff) {
			continue
		}
		kept = append(kept, e)
		delivered, failed = tally(e, delivered, failed)
	}
	c.events[queueID] = kept
	return delivered, failed
}

func tally(e event, delivered, failed float64) (float64, float64) {
	if e.delivered {
		return delivered + 1, failed
	}
	return delivered, failed + 1
}

func previews(dead []model.DeadLetter) []queuedapi.DeadLetter {
	result := make([]queuedapi.DeadLetter, 0, len(dead))
	for _, d := range dead {
		body := d.Body[:min(len(d.Body), previewBytes)]
		result = append(result, queuedapi.DeadLetter{
			ID: d.ID, Attempts: d.Attempts, LastStatus: d.LastStatus, LastError: d.LastError,
			PublishedAt: d.PublishedAt, BodyPreview: string(body),
		})
	}
	return result
}
