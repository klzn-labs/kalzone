package web

import (
	"fmt"
	"html/template"
	"math"
	"strings"

	observe "github.com/klzn-labs/kalzone/app/observe/entities"
)

const (
	chartWidth  = 640
	chartHeight = 80
	chartPad    = 6
)

func MetricChart(r observe.Reading) template.HTML {
	if len(r.Series) < 2 {
		return ""
	}
	top := math.Max(r.Peak, float64(r.Limit)) * 1.15
	y := func(v float64) float64 { return chartPad + (chartHeight-2*chartPad)*(1-v/top) }
	limit := y(float64(r.Limit))
	svg := fmt.Sprintf(`<svg class="metric-chart" viewBox="0 0 %d %d" preserveAspectRatio="none" role="img" aria-label="%s over the last 24 hours">`, chartWidth, chartHeight, r.Label())
	svg += fmt.Sprintf(`<line class="limit" x1="0" x2="%d" y1="%.1f" y2="%.1f"/>`, chartWidth, limit, limit)
	svg += fmt.Sprintf(`<path class="series" d="%s"/></svg>`, chartPath(r.Series, y))
	return template.HTML(svg)
}

func chartPath(series []observe.Sample, y func(float64) float64) string {
	steps := make([]string, 0, len(series))
	last := float64(len(series) - 1)
	for i, s := range series {
		op := "L"
		if i == 0 {
			op = "M"
		}
		steps = append(steps, fmt.Sprintf("%s%.1f %.1f", op, chartWidth*float64(i)/last, y(s.Value)))
	}
	return strings.Join(steps, " ")
}

func Rounded(v float64) int {
	return int(math.Round(v))
}
