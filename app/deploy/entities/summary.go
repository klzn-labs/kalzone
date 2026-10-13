package entities

import (
	"regexp"
	"time"
)

const (
	SummaryTail   = 20
	SummaryWindow = 500
)

var progressLine = regexp.MustCompile(`^\W*[0-9a-f]{12}\b`)

type Phase struct {
	Name    string
	Seconds int
}

type Summary struct {
	Total         int
	ProgressLines int
	Phases        []Phase
	Tail          []Event
}

func Summarize(d Deployment, recent []Event, total int, now time.Time) Summary {
	s := Summary{Total: total, Phases: d.Phases(now), Tail: []Event{}}
	for _, e := range recent {
		if progressLine.MatchString(e.Message) {
			s.ProgressLines++
			continue
		}
		s.Tail = append(s.Tail, e)
	}
	s.Tail = s.Tail[max(0, len(s.Tail)-SummaryTail):]
	return s
}

func (d Deployment) Phases(now time.Time) []Phase {
	waiting := Phase{Name: "waiting", Seconds: seconds(d.CreatedAt, firstOf(now, d.StartedAt, d.FinishedAt))}
	if d.StartedAt == nil {
		return []Phase{waiting}
	}
	running := Phase{Name: "running", Seconds: seconds(*d.StartedAt, firstOf(now, d.FinishedAt))}
	return []Phase{waiting, running}
}

func firstOf(fallback time.Time, times ...*time.Time) time.Time {
	for _, t := range times {
		if t != nil {
			return *t
		}
	}
	return fallback
}

func seconds(from, to time.Time) int {
	return int(to.Sub(from).Round(time.Second) / time.Second)
}
