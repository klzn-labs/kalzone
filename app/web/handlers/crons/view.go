package crons

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"time"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/cronspec"
)

type Row struct {
	Cron     entities.Cron
	Service  string
	LastRun  entities.Run
	HasRun   bool
	RunBadge string
}

type RunRow struct {
	Run      entities.Run
	Badge    string
	Trigger  string
	Started  time.Time
	Duration string
	HTTP     string
}

type detailView struct {
	Row     Row
	Runs    []RunRow
	NextRun *time.Time
	Payload string
	CanEdit bool
	Edit    bool
}

type newView struct {
	Services []catalog.Service
}

func (h CronsHandler) row(ctx context.Context, c entities.Cron) Row {
	service, _ := h.names.SlugOf(ctx, c.ServiceID)
	run, ok, _ := h.crons.LatestRun(ctx, c)
	return Row{Cron: c, Service: service, LastRun: run, HasRun: ok, RunBadge: runBadge(run.Status)}
}

func runRows(runs []entities.Run) []RunRow {
	rows := make([]RunRow, 0, len(runs))
	for _, r := range runs {
		rows = append(rows, RunRow{Run: r, Badge: runBadge(r.Status), Trigger: trigger(r), Started: started(r), Duration: duration(r), HTTP: httpStatus(r)})
	}
	return rows
}

func runBadge(status string) string {
	switch status {
	case entities.RunSucceeded:
		return "badge-ok"
	case entities.RunFailed, entities.RunTimedOut:
		return "badge-danger"
	default:
		return "badge-outline"
	}
}

func trigger(r entities.Run) string {
	if r.Manual {
		return "on demand"
	}
	return "scheduled"
}

func started(r entities.Run) time.Time {
	if r.StartedAt.IsZero() {
		return r.ScheduledAt
	}
	return r.StartedAt
}

func duration(r entities.Run) string {
	if r.StartedAt.IsZero() || r.FinishedAt.IsZero() {
		return "—"
	}
	d := r.FinishedAt.Sub(r.StartedAt)
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	return d.Round(100 * time.Millisecond).String()
}

func httpStatus(r entities.Run) string {
	if r.LastStatus == 0 {
		return "—"
	}
	return strconv.Itoa(r.LastStatus)
}

func nextRun(c entities.Cron) *time.Time {
	schedule, err := cronspec.Parse(c.Schedule)
	if err != nil || c.Paused {
		return nil
	}
	next := schedule.Next(clock.Now().UTC())
	return &next
}

func prettyPayload(body []byte) string {
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		return string(body)
	}
	return out.String()
}
