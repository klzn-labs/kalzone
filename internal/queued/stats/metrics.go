package stats

import (
	"time"

	"github.com/adrianozp/platform/internal/queued/dispatch"
	"github.com/adrianozp/platform/internal/queued/model"
	"github.com/prometheus/client_golang/prometheus"
)

var outcomes = map[dispatch.Verdict]string{dispatch.Ack: "ack", dispatch.Retry: "retry", dispatch.Bury: "dead"}

type metrics struct {
	depth      *prometheus.GaugeVec
	inFlight   *prometheus.GaugeVec
	dead       *prometheus.GaugeVec
	deliveries *prometheus.CounterVec
	duration   *prometheus.HistogramVec
}

func newMetrics(r prometheus.Registerer) metrics {
	labels := []string{"queue", "environment"}
	m := metrics{
		depth:      prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "platform_queue_depth", Help: "Messages waiting, delayed or in flight."}, labels),
		inFlight:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "platform_queue_in_flight", Help: "Deliveries waiting for an answer."}, labels),
		dead:       prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "platform_queue_dead_letters", Help: "Messages in the dead-letter stream."}, labels),
		deliveries: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "platform_queue_delivered_total", Help: "Deliveries by outcome."}, append(labels, "outcome")),
		duration:   prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "platform_queue_delivery_seconds", Help: "Delivery round trips.", Buckets: prometheus.DefBuckets}, labels),
	}
	r.MustRegister(m.depth, m.inFlight, m.dead, m.deliveries, m.duration)
	return m
}

func (m metrics) delivered(labels prometheus.Labels, verdict dispatch.Verdict, took time.Duration) {
	withOutcome := prometheus.Labels{"queue": labels["queue"], "environment": labels["environment"], "outcome": outcomes[verdict]}
	m.deliveries.With(withOutcome).Inc()
	m.duration.With(labels).Observe(took.Seconds())
}

func (m metrics) observe(labels prometheus.Labels, b model.BackendStats) {
	m.depth.With(labels).Set(float64(b.Depth))
	m.inFlight.With(labels).Set(float64(b.InFlight))
	m.dead.With(labels).Set(float64(b.DeadLetters))
}
