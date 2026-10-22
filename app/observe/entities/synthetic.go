package entities

import (
	"fmt"
	"hash/fnv"
	"math"
	"time"
)

type shape struct {
	base   float64
	swing  float64
	perMin bool
}

var shapes = map[Signal]shape{
	SignalRequests: {base: 9, swing: 6, perMin: true},
	SignalErrors:   {base: 0.003, swing: 0.002},
	SignalLatency:  {base: 140, swing: 35},
	SignalMemory:   {base: 180, swing: 8},
}

var syntheticLines = []string{
	"GET /api/products?category=shoes 200 %dms",
	"GET /api/cart 200 %dms",
	"POST /api/cart/items 201 %dms",
	"GET /api/orders/recent 200 %dms",
	"POST /api/checkout 202 %dms",
	"GET /healthz 200 %dms",
}

func SyntheticSeries(s Signal, slug string, w Window, end time.Time) []Sample {
	points := int(w.Span / w.Step)
	samples := make([]Sample, 0, points)
	for i := range points {
		at := end.Add(-time.Duration(points-1-i) * w.Step).Truncate(time.Second)
		samples = append(samples, Sample{At: at, Value: syntheticValue(shapes[s], seedOf(slug), at, w.Step)})
	}
	return samples
}

func syntheticValue(sh shape, seed float64, at time.Time, step time.Duration) float64 {
	hour := float64(at.Hour()) + float64(at.Minute())/60
	daily := math.Sin((hour-9)/24*2*math.Pi + seed)
	wobble := math.Sin(float64(at.Unix())/900+seed*3) * 0.25
	value := sh.base + sh.swing*(daily+wobble+jitter(seed, at))
	if sh.perMin {
		value *= step.Minutes()
	}
	return math.Max(value, 0)
}

func jitter(seed float64, at time.Time) float64 {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%.2f-%d", seed, at.Unix())
	return float64(h.Sum32()%1000)/1000*0.8 - 0.4
}

func seedOf(slug string) float64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(slug))
	return float64(h.Sum32()%628) / 100
}

func SyntheticLines(slug string, end time.Time, limit int) []LogLine {
	count := min(limit, 30)
	lines := make([]LogLine, 0, count)
	for i := range count {
		at := end.Add(-time.Duration(count-i) * 47 * time.Second)
		text := fmt.Sprintf(syntheticLines[i%len(syntheticLines)], 12+(i*37)%90)
		if i%11 == 7 {
			text = "warn: payment provider answered slowly, retrying once"
		}
		lines = append(lines, LogLine{At: at, Container: slug + "-green", Line: text})
	}
	return lines
}
