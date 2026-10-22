package entities

import (
	"fmt"
	"time"
)

type Signal string

const (
	SignalRequests Signal = "requests"
	SignalErrors   Signal = "errors"
	SignalLatency  Signal = "latency"
	SignalMemory   Signal = "memory"
)

var Signals = []Signal{SignalRequests, SignalErrors, SignalLatency, SignalMemory}

func SignalNamed(name string) Signal {
	for _, s := range Signals {
		if string(s) == name {
			return s
		}
	}
	return SignalLatency
}

type Window struct {
	Name string
	Span time.Duration
	Step time.Duration
}

var Windows = []Window{
	{Name: "1h", Span: time.Hour, Step: time.Minute},
	{Name: "6h", Span: 6 * time.Hour, Step: 5 * time.Minute},
	{Name: "24h", Span: 24 * time.Hour, Step: 15 * time.Minute},
	{Name: "7d", Span: 7 * 24 * time.Hour, Step: 2 * time.Hour},
}

func WindowNamed(name string) Window {
	for _, w := range Windows {
		if w.Name == name {
			return w
		}
	}
	return Windows[1]
}

type Metrics struct {
	Window Window
	End    time.Time
	Series map[Signal][]Sample
}

func (m Metrics) Start() time.Time {
	return m.End.Add(-m.Window.Span)
}

func SignalQuery(s Signal, slug, environment string, step time.Duration) string {
	traefik := fmt.Sprintf(`service=~"%s-%s-(blue|green)@file"`, slug, environment)
	window := fmt.Sprintf("%dm", int(step.Minutes()))
	switch s {
	case SignalRequests:
		return fmt.Sprintf("sum(increase(traefik_service_requests_total{%s}[%s]))", traefik, window)
	case SignalErrors:
		return fmt.Sprintf(`sum(rate(traefik_service_requests_total{%s,code=~"5.."}[%s])) / sum(rate(traefik_service_requests_total{%s}[%s]))`, traefik, window, traefik, window)
	case SignalLatency:
		return fmt.Sprintf("histogram_quantile(0.95, sum by (le) (rate(traefik_service_request_duration_seconds_bucket{%s}[%s]))) * 1000", traefik, window)
	}
	return fmt.Sprintf(`sum(container_memory_working_set_bytes{name=~"%s-%s-(blue|green)"}) / 1048576`, slug, environment)
}
