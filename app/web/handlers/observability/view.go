package observability

import (
	"net/url"
	"regexp"
	"slices"

	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/observe/entities"
)

var (
	errorLine = regexp.MustCompile(`(?i)error|panic|fatal|exception|\s5\d\d\s`)
	warnLine  = regexp.MustCompile(`(?i)warn`)
)

type Tile struct {
	Label    string
	Value    string
	Note     string
	Spark    string
	Href     string
	Hot      bool
	Selected bool
}

type Chart struct {
	Title    string
	Note     string
	Path     string
	LimitY   float64
	HasLimit bool
	Markers  []Marker
	Ticks    []string
	Axis     []string
	Empty    bool
}

type Link struct {
	Label  string
	Href   string
	Active bool
}

type LogRow struct {
	At   string
	Line string
	Tone string
}

type View struct {
	Tiles     []Tile
	Chart     Chart
	Windows   []Link
	LogModes  []Link
	Logs      []LogRow
	LogsNote  string
	Dashboard string
}

type state struct {
	base   string
	window entities.Window
	signal entities.Signal
	logs   string
}

func (s state) href(window, signal, logs string) string {
	return s.base + "?" + url.Values{"window": {window}, "signal": {signal}, "logs": {logs}}.Encode()
}

type inputs struct {
	state   state
	metrics entities.Metrics
	limits  limits
	history []deploy.Deployment
}

func viewOf(in inputs) View {
	return View{Tiles: tilesOf(in), Chart: chartOf(in), Windows: windowLinks(in.state), LogModes: logLinks(in.state)}
}

func tilesOf(in inputs) []Tile {
	tiles := make([]Tile, 0, len(entities.Signals))
	for _, s := range entities.Signals {
		tiles = append(tiles, tileOf(in, s))
	}
	return tiles
}

func tileOf(in inputs, s entities.Signal) Tile {
	samples := in.metrics.Series[s]
	r := readingOf(samples)
	f := frame{start: in.metrics.Start(), span: in.metrics.Window.Span, width: sparkWidth, height: sparkHeight, top: topOf(r.peak, 0)}
	return Tile{
		Label: labels[s], Value: headline(s, r), Note: note(s, r, in.limits, in.metrics.Window), Spark: f.path(samples),
		Href: in.state.href(in.state.window.Name, string(s), in.state.logs), Hot: hot(s, r, in.limits), Selected: s == in.state.signal,
	}
}

func chartOf(in inputs) Chart {
	s := in.state.signal
	samples := in.metrics.Series[s]
	r, limit := readingOf(samples), in.limits.of(s)
	f := frame{start: in.metrics.Start(), span: in.metrics.Window.Span, width: chartWidth, height: chartHeight, top: topOf(r.peak, limit)}
	c := Chart{
		Title: labels[s], Note: chartNote(s, r, limit), Path: f.path(samples), Markers: markersOf(f, in.history),
		Ticks: []string{format(s, f.top), format(s, f.top/2), format(s, 0)}, Axis: axisOf(f), Empty: !r.known,
	}
	if limit > 0 {
		c.HasLimit, c.LimitY = true, f.y(limit)
	}
	return c
}

func chartNote(s entities.Signal, r reading, limit float64) string {
	if !r.known {
		return "No data in this window"
	}
	text := "peak " + format(s, r.peak) + " at " + r.peakAt.UTC().Format("15:04")
	if limit > 0 {
		text += " · limit " + format(s, limit)
	}
	return text
}

func windowLinks(s state) []Link {
	links := make([]Link, 0, len(entities.Windows))
	for _, w := range entities.Windows {
		links = append(links, Link{Label: w.Name, Href: s.href(w.Name, string(s.signal), s.logs), Active: w.Name == s.window.Name})
	}
	return links
}

func logLinks(s state) []Link {
	return []Link{
		{Label: "All lines", Href: s.href(s.window.Name, string(s.signal), "all"), Active: s.logs != "errors"},
		{Label: "Errors", Href: s.href(s.window.Name, string(s.signal), "errors"), Active: s.logs == "errors"},
	}
}

func logRows(lines []entities.LogLine) []LogRow {
	rows := make([]LogRow, 0, len(lines))
	for _, l := range slices.Backward(lines) {
		rows = append(rows, LogRow{At: l.At.UTC().Format("15:04:05"), Line: l.Line, Tone: toneOf(l.Line)})
	}
	return rows
}

func toneOf(line string) string {
	switch {
	case errorLine.MatchString(line):
		return "error"
	case warnLine.MatchString(line):
		return "warn"
	}
	return ""
}
