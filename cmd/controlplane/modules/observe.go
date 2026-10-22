package modules

import (
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/services"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	logsHandler "github.com/klzn-labs/kalzone/app/observe/handlers/logs"
	"github.com/klzn-labs/kalzone/app/observe/handlers/proxy"
	"github.com/klzn-labs/kalzone/app/observe/handlers/sync"
	observeCatalog "github.com/klzn-labs/kalzone/app/observe/repositories/catalog"
	"github.com/klzn-labs/kalzone/app/observe/repositories/grafana"
	"github.com/klzn-labs/kalzone/app/observe/repositories/pushgateway"
	"github.com/klzn-labs/kalzone/app/observe/usecases/dashboards"
	"github.com/klzn-labs/kalzone/app/observe/usecases/health"
	"github.com/klzn-labs/kalzone/app/observe/usecases/logs"
	"github.com/klzn-labs/kalzone/app/observe/usecases/metrics"
	"github.com/klzn-labs/kalzone/internal/eventbus"
	"github.com/klzn-labs/kalzone/pkg/config"
	"go.uber.org/fx"
)

func Observe() fx.Option {
	return fx.Options(
		fx.Provide(
			grafana.NewClient, dashboards.NewDashboardsUsecase, proxy.NewGrafanaProxy,
			func(c grafana.Client) dashboards.Grafana { return c },
			pushgateway.NewClient,
			func(c pushgateway.Client) dashboards.Publisher { return c },
			func(u services.ServicesUsecase, d databases.DatabasesUsecase) dashboards.Catalog {
				return observeCatalog.NewServices(u, d)
			},
			func(u dashboards.DashboardsUsecase) sync.Syncer { return u },
			sync.NewDefaultWorker,
			func(c grafana.Client, cfg config.Config) health.HealthUsecase { return health.NewHealthUsecase(c, cfg) },
		),
		observeLogs(),
		fx.Invoke(proxy.RegisterProxyRoutes, sync.OnStart, logsHandler.RegisterLogsRoutes, subscribeDashboards),
	)
}

func subscribeDashboards(b *eventbus.Bus, w *sync.Worker) {
	b.Subscribe(w)
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
		func(l observeCatalog.Lookup, c grafana.Client, cfg config.Config) metrics.MetricsUsecase {
			return metrics.NewMetricsUsecase(l, c, cfg.Grafana.PrometheusUID)
		},
	)
}
