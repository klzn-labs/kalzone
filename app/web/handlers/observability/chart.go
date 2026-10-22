package observability

import (
	"fmt"
	"strings"
	"time"

	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/observe/entities"
)

const (
	chartWidth  = 640.0
	chartHeight = 180.0
	sparkWidth  = 120.0
	sparkHeight = 28.0
	headroom    = 1.15
)

type frame struct {
	start  time.Time
	span   time.Duration
	width  float64
	height float64
	top    float64
}

func (f frame) x(at time.Time) float64 {
	return float64(at.Sub(f.start)) / float64(f.span) * f.width
}

func (f frame) y(value float64) float64 {
	return f.height - value/f.top*f.height
}

func (f frame) path(samples []entities.Sample) string {
	points := make([]string, 0, len(samples))
	for _, s := range samples {
		points = append(points, fmt.Sprintf("%.1f,%.1f", f.x(s.At), f.y(s.Value)))
	}
	if len(points) == 0 {
		return ""
	}
	return "M" + strings.Join(points, " L")
}

func topOf(peak, limit float64) float64 {
	top := max(peak, limit) * headroom
	if top <= 0 {
		return 1
	}
	return top
}

type Marker struct {
	X     float64
	Label string
}

func markersOf(f frame, history []deploy.Deployment) []Marker {
	markers := []Marker{}
	end := f.start.Add(f.span)
	for _, d := range history {
		if d.FinishedAt == nil || d.FinishedAt.Before(f.start) || d.FinishedAt.After(end) {
			continue
		}
		markers = append(markers, Marker{X: f.x(*d.FinishedAt), Label: fmt.Sprintf("r%d · %s", d.ReleaseNumber, d.FinishedAt.UTC().Format("15:04"))})
	}
	return markers
}

func axisOf(f frame) []string {
	layout := "15:04"
	if f.span > 24*time.Hour {
		layout = "Jan 2"
	}
	labels := make([]string, 0, 4)
	for i := range 4 {
		labels = append(labels, f.start.Add(f.span*time.Duration(i)/3).UTC().Format(layout))
	}
	return labels
}
