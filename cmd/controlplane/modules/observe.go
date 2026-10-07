package modules

import (
	"github.com/adrianozp/platform/app/catalog/usecases/services"
	"github.com/adrianozp/platform/app/observe/handlers/proxy"
	"github.com/adrianozp/platform/app/observe/handlers/sync"
	"github.com/adrianozp/platform/app/observe/repositories/grafana"
	"github.com/adrianozp/platform/app/observe/usecases/dashboards"
	"github.com/adrianozp/platform/pkg/events"
	"go.uber.org/fx"
)

func Observe() fx.Option {
	return fx.Options(
		fx.Provide(
			grafana.NewClient, dashboards.NewDashboardsUsecase, proxy.NewGrafanaProxy,
			func(c grafana.Client) dashboards.Grafana { return c },
			func(u services.ServicesUsecase) sync.Services { return u },
			func(u dashboards.DashboardsUsecase) sync.Writer { return u },
			fx.Annotate(func(u dashboards.DashboardsUsecase) events.Subscriber { return u }, fx.ResultTags(`group:"subscribers"`)),
		),
		fx.Invoke(proxy.RegisterProxyRoutes, sync.OnStart),
	)
}
