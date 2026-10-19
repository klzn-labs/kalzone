package modules

import (
	requests "github.com/klzn-labs/kalzone/app/approvals/usecases/requests"
	"github.com/klzn-labs/kalzone/app/authz"
	environmentsHandler "github.com/klzn-labs/kalzone/app/catalog/handlers/environments"
	servicesHandler "github.com/klzn-labs/kalzone/app/catalog/handlers/services"
	"github.com/klzn-labs/kalzone/app/catalog/repositories/mysql"
	"github.com/klzn-labs/kalzone/app/catalog/repositories/teams"
	"github.com/klzn-labs/kalzone/app/catalog/repositories/teardown"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/decommission"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/environments"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/ownership"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/services"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/deployments"
	"github.com/klzn-labs/kalzone/app/observe/usecases/dashboards"
	queues "github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/app/release/usecases/releases"
	teamsUsecase "github.com/klzn-labs/kalzone/app/team/usecases/teams"
	"github.com/klzn-labs/kalzone/internal/eventbus"
	"go.uber.org/fx"
)

func Catalog() fx.Option {
	return fx.Options(
		fx.Provide(
			mysql.NewServiceRepository, mysql.NewEnvironmentRepository, teams.NewLookup,
			services.NewServicesUsecase, environments.NewEnvironmentsUsecase, ownership.NewOwnershipUsecase,
			servicesHandler.NewServicesHandler, environmentsHandler.NewEnvironmentsHandler,
		),
		catalogInjections(),
		decommissionInjections(),
		fx.Invoke(servicesHandler.RegisterServicesRoutes, environmentsHandler.RegisterEnvironmentsRoutes, subscribeDecommission),
	)
}

func decommissionInjections() fx.Option {
	return fx.Provide(
		decommission.NewDecommissionUsecase, teardown.NewHost,
		func(u services.ServicesUsecase) decommission.Services { return u },
		func(h teardown.Host) decommission.Host { return h },
		func(u deployments.DeploymentsUsecase) teardown.Deployments { return u },
		func(u releases.ReleasesUsecase) decommission.Releases { return u },
		func(u requests.RequestsUsecase) decommission.Requests { return u },
		func(u queues.QueuesUsecase) decommission.Queues { return u },
		func(u databases.DatabasesUsecase) decommission.Databases { return u },
		func(u dashboards.DashboardsUsecase) decommission.Dashboards { return u },
		func(u decommission.DecommissionUsecase) servicesHandler.Decommissioner { return u },
	)
}

func subscribeDecommission(b *eventbus.Bus, u decommission.DecommissionUsecase) {
	b.Subscribe(u)
}

func catalogInjections() fx.Option {
	return fx.Provide(
		func(r mysql.ServiceRepository) services.ServiceStore { return r },
		func(r mysql.EnvironmentRepository) environments.EnvironmentStore { return r },
		func(r mysql.ServiceRepository) ownership.ServiceCounter { return r },
		func(u teamsUsecase.TeamsUsecase) teams.TeamFinder { return u },
		func(l teams.Lookup) services.TeamLookup { return l },
		func(p authz.Policy) services.Authorizer { return p },
		func(b *eventbus.Bus) services.Publisher { return b },
		func(u services.ServicesUsecase) servicesHandler.Services { return u },
		func(u environments.EnvironmentsUsecase) environmentsHandler.Environments { return u },
	)
}
