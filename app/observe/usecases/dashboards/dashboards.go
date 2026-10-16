package dashboards

import (
	"context"
	"fmt"
	"log"

	"github.com/adrianozp/platform/app/observe/entities"
	"github.com/adrianozp/platform/pkg/events"
)

type Grafana interface {
	Enabled() bool
	EnsureFolder(ctx context.Context) error
	UpsertDashboard(ctx context.Context, dashboard map[string]any) error
	DeleteDashboard(ctx context.Context, uid string) error
}

type DashboardsUsecase struct {
	grafana Grafana
}

func NewDashboardsUsecase(grafana Grafana) DashboardsUsecase {
	return DashboardsUsecase{grafana: grafana}
}

func (u DashboardsUsecase) Write(ctx context.Context, services []entities.Service) (int, error) {
	if !u.grafana.Enabled() || len(services) == 0 {
		return 0, nil
	}
	if err := u.grafana.EnsureFolder(ctx); err != nil {
		return 0, err
	}
	for i, s := range services {
		if err := u.grafana.UpsertDashboard(ctx, entities.Dashboard(s)); err != nil {
			return i, fmt.Errorf("dashboard for %s: %w", s.Slug, err)
		}
	}
	return len(services), nil
}

func (u DashboardsUsecase) Delete(ctx context.Context, serviceSlug string) error {
	if !u.grafana.Enabled() {
		return nil
	}
	return u.grafana.DeleteDashboard(ctx, entities.DashboardUID(serviceSlug))
}

func (u DashboardsUsecase) Handle(ctx context.Context, e events.Event) error {
	if e.Action != "service.created" && e.Action != "service.updated" {
		return nil
	}
	name, _ := e.Details["name"].(string)
	_, err := u.Write(ctx, []entities.Service{{Slug: e.TargetName, Name: name}})
	if err != nil {
		log.Printf("dashboards: %v", err)
	}
	return err
}
