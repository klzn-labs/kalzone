package observability

import (
	"fmt"
	"time"

	"github.com/klzn-labs/kalzone/app/observe/entities"
)

const (
	errorsHot = 0.01
	memoryHot = 0.9
)

var labels = map[entities.Signal]string{
	entities.SignalRequests: "Requests", entities.SignalErrors: "5xx ratio",
	entities.SignalLatency: "p95 latency", entities.SignalMemory: "Memory",
}

type limits struct {
	latencyMS float64
	memoryMB  float64
}

func (l limits) of(s entities.Signal) float64 {
	switch s {
	case entities.SignalLatency:
		return l.latencyMS
	case entities.SignalMemory:
		return l.memoryMB
	}
	return 0
}

type reading struct {
	known  bool
	now    float64
	peak   float64
	peakAt time.Time
	total  float64
}

func readingOf(samples []entities.Sample) reading {
	r := reading{known: len(samples) > 0}
	for _, s := range samples {
		r.total += s.Value
		r.now = s.Value
		if s.Value >= r.peak {
			r.peak, r.peakAt = s.Value, s.At
		}
	}
	return r
}

func format(s entities.Signal, v float64) string {
	switch s {
	case entities.SignalRequests:
		return thousands(int(v + 0.5))
	case entities.SignalErrors:
		return fmt.Sprintf("%.1f%%", v*100)
	case entities.SignalLatency:
		return fmt.Sprintf("%.0f ms", v)
	}
	return fmt.Sprintf("%.0f MB", v)
}

func thousands(n int) string {
	digits := fmt.Sprint(n)
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	return digits
}

func headline(s entities.Signal, r reading) string {
	if !r.known {
		return "—"
	}
	if s == entities.SignalRequests {
		return format(s, r.total)
	}
	return format(s, r.now)
}

func note(s entities.Signal, r reading, l limits, w entities.Window) string {
	switch {
	case !r.known:
		return "no data"
	case s == entities.SignalRequests:
		return "in " + w.Name
	case s == entities.SignalErrors:
		return "peak " + format(s, r.peak)
	case l.of(s) > 0:
		return "limit " + format(s, l.of(s))
	}
	return "peak " + format(s, r.peak)
}

func hot(s entities.Signal, r reading, l limits) bool {
	switch s {
	case entities.SignalErrors:
		return r.peak >= errorsHot
	case entities.SignalLatency:
		return l.latencyMS > 0 && r.peak > l.latencyMS
	case entities.SignalMemory:
		return l.memoryMB > 0 && r.peak > l.memoryMB*memoryHot
	}
	return false
}
