package dashboards

import (
	"context"
	"fmt"

	"github.com/klzn-labs/kalzone/app/observe/entities"
)

type Grafana interface {
	Enabled() bool
	EnsureFolder(ctx context.Context) error
	UpsertDashboard(ctx context.Context, dashboard map[string]any) error
	DeleteDashboard(ctx context.Context, uid string) error
}

type Catalog interface {
	Services(ctx context.Context) ([]entities.Service, error)
}

type Publisher interface {
	Enabled() bool
	PublishAttachments(ctx context.Context, services []entities.Service) error
}

type DashboardsUsecase struct {
	grafana   Grafana
	catalog   Catalog
	publisher Publisher
}

func NewDashboardsUsecase(grafana Grafana, catalog Catalog, publisher Publisher) DashboardsUsecase {
	return DashboardsUsecase{grafana: grafana, catalog: catalog, publisher: publisher}
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

func (u DashboardsUsecase) SyncsOn(action string) bool {
	return syncedOn[action]
}
