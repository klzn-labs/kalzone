package crons

import (
	"context"

	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Crons interface {
	ListFor(ctx context.Context, a actor.Actor, environment, teamSlug string) ([]entities.Cron, error)
	LatestRun(ctx context.Context, c entities.Cron) (entities.Run, bool, error)
}

type ServiceNames interface {
	SlugOf(ctx context.Context, id string) (string, error)
}

type Row struct {
	Cron     entities.Cron
	Service  string
	LastRun  entities.Run
	HasRun   bool
	RunBadge string
}

type CronsHandler struct {
	crons Crons
	names ServiceNames
}

func NewCronsHandler(crons Crons, names ServiceNames) CronsHandler {
	return CronsHandler{crons: crons, names: names}
}

func (h CronsHandler) Rows(ctx context.Context, a actor.Actor, environment string) ([]Row, error) {
	list, err := h.crons.ListFor(ctx, a, environment, "")
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(list))
	for _, c := range list {
		rows = append(rows, h.row(ctx, c))
	}
	return rows, nil
}

func (h CronsHandler) row(ctx context.Context, c entities.Cron) Row {
	service, _ := h.names.SlugOf(ctx, c.ServiceID)
	run, ok, _ := h.crons.LatestRun(ctx, c)
	return Row{Cron: c, Service: service, LastRun: run, HasRun: ok, RunBadge: runBadge(run.Status)}
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
