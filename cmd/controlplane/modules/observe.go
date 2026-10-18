package modules

import (
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/services"
	logsHandler "github.com/klzn-labs/kalzone/app/observe/handlers/logs"
	"github.com/klzn-labs/kalzone/app/observe/handlers/proxy"
	"github.com/klzn-labs/kalzone/app/observe/handlers/sync"
	observeCatalog "github.com/klzn-labs/kalzone/app/observe/repositories/catalog"
	"github.com/klzn-labs/kalzone/app/observe/repositories/grafana"
	"github.com/klzn-labs/kalzone/app/observe/usecases/dashboards"
	"github.com/klzn-labs/kalzone/app/observe/usecases/logs"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/events"
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
		observeLogs(),
		fx.Invoke(proxy.RegisterProxyRoutes, sync.OnStart, logsHandler.RegisterLogsRoutes),
	)
}

func observeLogs() fx.Option {
	return fx.Provide(
		logsHandler.NewLogsHandler,
		func(u services.ServicesUsecase) observeCatalog.ServiceFinder { return u },
		observeCatalog.NewLookup,
		func(l observeCatalog.Lookup, p authz.Policy, c grafana.Client, cfg config.Config) logs.LogsUsecase {
			return logs.NewLogsUsecase(l, p, c, cfg.Grafana.LokiUID)
		},
		func(u logs.LogsUsecase) logsHandler.Logs { return u },
	)
}
