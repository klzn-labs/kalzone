package governor

import (
	"github.com/adrianozp/gaardrail/pkg/control"
	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	factor       prometheus.Gauge
	measured     prometheus.Gauge
	setpoint     prometheus.Gauge
	failSafe     prometheus.Gauge
	terms        *prometheus.GaugeVec
	readFailures prometheus.Counter
	transitions  *prometheus.CounterVec
}

func newMetrics(r prometheus.Registerer) metrics {
	m := metrics{
		factor:       gauge("queued_adaptive_factor", "Share of the way from each adaptive queue's minimum to its rate, 0 to 1."),
		measured:     gauge("queued_adaptive_measured", "The last database CPU read, in percent of one core."),
		setpoint:     gauge("queued_adaptive_setpoint", "The database CPU the governor holds, in percent of one core."),
		failSafe:     gauge("queued_adaptive_failsafe", "1 while measurements are missing and adaptive queues return to their fixed rate."),
		terms:        prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "queued_adaptive_term", Help: "Proportional and integral parts of the throttle."}, []string{"term"}),
		readFailures: prometheus.NewCounter(prometheus.CounterOpts{Name: "queued_adaptive_read_failures_total", Help: "Measurements that failed, were empty or were too old."}),
		transitions:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: "queued_adaptive_failsafe_transitions_total", Help: "Moves into and out of fail-safe."}, []string{"direction"}),
	}
	r.MustRegister(m.factor, m.measured, m.setpoint, m.failSafe, m.terms, m.readFailures, m.transitions)
	return m
}

func gauge(name, help string) prometheus.Gauge {
	return prometheus.NewGauge(prometheus.GaugeOpts{Name: name, Help: help})
}

func (m metrics) show(s model.AdaptiveState, t control.Terms) {
	m.factor.Set(s.Factor)
	m.measured.Set(s.Measured)
	m.setpoint.Set(s.Setpoint)
	m.failSafe.Set(boolValue(s.FailSafe))
	m.terms.WithLabelValues("p").Set(t.P)
	m.terms.WithLabelValues("i").Set(t.I)
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
