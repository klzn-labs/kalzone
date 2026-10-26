package governor

import (
	"context"
	"errors"
	"log"
	"math"
	"sync"
	"time"

	"github.com/adrianozp/gaardrail/pkg/control"
	"github.com/adrianozp/gaardrail/pkg/control/filter"
	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/prometheus/client_golang/prometheus"
)

const recoveryStep = 0.25

var (
	errNotANumber = errors.New("governor: the measurement is not a number")
	errStale      = errors.New("governor: the measurement is older than two periods")
)

type Sample struct {
	Value float64
	At    time.Time
}

type Reader interface {
	Read(ctx context.Context) (Sample, error)
}

type Settings struct {
	Setpoint   float64
	Kp         float64
	Ki         float64
	Period     time.Duration
	StaleAfter int
	FilterSize int
}

type Governor struct {
	reader   Reader
	settings Settings
	now      func() time.Time
	pi       *control.PID
	filter   *filter.Signal
	metrics  metrics
	mu       sync.Mutex
	state    model.AdaptiveState
	failures int
}

func New(reader Reader, s Settings, now func() time.Time, r prometheus.Registerer) (*Governor, error) {
	pi, err := control.NewPID(throttleParams(s))
	if err != nil {
		return nil, err
	}
	measurement, err := filter.NewMeasurement(measurementSpec(s.FilterSize))
	if err != nil {
		return nil, err
	}
	g := &Governor{reader: reader, settings: s, now: now, pi: pi, filter: measurement, metrics: newMetrics(r)}
	g.state = model.AdaptiveState{On: true, Factor: 1, Setpoint: s.Setpoint}
	g.metrics.show(g.state, control.Terms{})
	return g, nil
}

func throttleParams(s Settings) control.PIDParams {
	return control.PIDParams{
		Kp: -s.Kp, Ki: -s.Ki, Setpoint: s.Setpoint,
		Limits: control.Limits{Min: 0, Max: 1, IClamp: 1},
	}
}

func measurementSpec(size int) filter.Spec {
	if size <= 1 {
		return filter.Spec{Kind: filter.None}
	}
	return filter.Spec{Kind: filter.MovingAverage, Size: size}
}

func (g *Governor) Adaptive() model.AdaptiveState {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state
}

func (g *Governor) Run(ctx context.Context) {
	ticker := time.NewTicker(g.settings.Period)
	defer ticker.Stop()
	for {
		g.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (g *Governor) Tick(ctx context.Context) {
	sample, err := g.reader.Read(ctx)
	if err == nil {
		err = g.check(sample)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if err != nil {
		g.miss(err)
		return
	}
	g.hit(sample)
}

func (g *Governor) check(s Sample) error {
	if math.IsNaN(s.Value) || math.IsInf(s.Value, 0) {
		return errNotANumber
	}
	if g.now().Sub(s.At) > 2*g.settings.Period {
		return errStale
	}
	return nil
}

func (g *Governor) miss(err error) {
	g.failures++
	g.metrics.readFailures.Inc()
	if g.failures >= g.settings.StaleAfter {
		g.enterFailSafe(err)
		g.state.Factor = math.Min(1, g.state.Factor+recoveryStep)
	}
	g.metrics.show(g.state, control.Terms{})
}

func (g *Governor) enterFailSafe(err error) {
	if g.state.FailSafe {
		return
	}
	g.state.FailSafe = true
	g.pi.Reset()
	g.filter.Reset()
	g.metrics.transitions.WithLabelValues("enter").Inc()
	log.Printf("adaptive rates: fail-safe after %d bad reads (%v); adaptive queues return to their fixed rate", g.failures, err)
}

func (g *Governor) leaveFailSafe() {
	if !g.state.FailSafe {
		return
	}
	g.state.FailSafe = false
	g.metrics.transitions.WithLabelValues("leave").Inc()
	log.Printf("adaptive rates: measurements are back; the governor starts again")
}

func (g *Governor) hit(s Sample) {
	measured := g.filter.Filter(s.Value)
	throttle, terms, err := g.pi.Compute(measured, s.At)
	if err != nil {
		g.miss(err)
		return
	}
	g.failures = 0
	g.leaveFailSafe()
	g.state.Factor, g.state.Measured = 1-throttle, s.Value
	g.metrics.show(g.state, terms)
}
