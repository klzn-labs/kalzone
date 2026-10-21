package entities

import (
	"fmt"
	"time"
)

type Metric string

const (
	MetricResponse Metric = "response"
	MetricCPU      Metric = "cpu"
	MetricDBCPU    Metric = "db_cpu"
)

var metricNames = map[Metric][2]string{
	MetricResponse: {"Response p95", " ms"},
	MetricCPU:      {"CPU", "%"},
	MetricDBCPU:    {"DB CPU", "%"},
}

type Reading struct {
	Metric  Metric
	Limit   int
	Now     float64
	Known   bool
	Peak    float64
	PeakAt  time.Time
	OverFor time.Duration
	Series  []Sample
}

func (r Reading) Label() string { return metricNames[r.Metric][0] }

func (r Reading) Unit() string { return metricNames[r.Metric][1] }

func (r Reading) OverNow() bool { return r.Known && r.Now >= float64(r.Limit) }

func (r Reading) WentOver() bool { return r.Known && (r.OverNow() || r.Peak >= float64(r.Limit)) }

func (r Reading) Problem() string {
	return fmt.Sprintf("%s over %d%s", r.Label(), r.Limit, r.Unit())
}

func ReadingOf(metric Metric, limit int, samples []Sample, step time.Duration) Reading {
	r := Reading{Metric: metric, Limit: limit, Series: samples, Known: len(samples) > 0}
	for _, s := range samples {
		if s.Value >= r.Peak {
			r.Peak, r.PeakAt = s.Value, s.At
		}
		if s.Value >= float64(limit) {
			r.OverFor += step
		}
	}
	if r.Known {
		r.Now = samples[len(samples)-1].Value
	}
	return r
}

type Health struct {
	Readings []Reading
}

func (h Health) Known() bool {
	return len(h.filter(func(r Reading) bool { return r.Known })) > 0
}

func (h Health) Over() []Reading {
	return h.filter(Reading.OverNow)
}

func (h Health) WentOver() []Reading {
	return h.filter(Reading.WentOver)
}

func (h Health) Calm() []Reading {
	return h.filter(func(r Reading) bool { return !r.WentOver() })
}

func (h Health) filter(keep func(Reading) bool) []Reading {
	kept := []Reading{}
	for _, r := range h.Readings {
		if keep(r) {
			kept = append(kept, r)
		}
	}
	return kept
}

type Delivery struct {
	FailedBuild  int
	FailedDeploy int
}

func Problems(h Health, d Delivery) []string {
	problems := []string{}
	for _, r := range h.Over() {
		problems = append(problems, r.Problem())
	}
	if d.FailedBuild > 0 {
		problems = append(problems, fmt.Sprintf("Build #%d failed", d.FailedBuild))
	}
	if d.FailedDeploy > 0 {
		problems = append(problems, fmt.Sprintf("Deploy #%d to prod failed", d.FailedDeploy))
	}
	return problems
}
