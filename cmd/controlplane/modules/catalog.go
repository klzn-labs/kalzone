package modules

import (
	"github.com/adrianozp/platform/app/authz"
	environmentsHandler "github.com/adrianozp/platform/app/catalog/handlers/environments"
	servicesHandler "github.com/adrianozp/platform/app/catalog/handlers/services"
	"github.com/adrianozp/platform/app/catalog/repositories/mysql"
	"github.com/adrianozp/platform/app/catalog/repositories/teams"
	"github.com/adrianozp/platform/app/catalog/usecases/environments"
	"github.com/adrianozp/platform/app/catalog/usecases/ownership"
	"github.com/adrianozp/platform/app/catalog/usecases/services"
	teamsUsecase "github.com/adrianozp/platform/app/team/usecases/teams"
	"github.com/adrianozp/platform/internal/eventbus"
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
		fx.Invoke(servicesHandler.RegisterServicesRoutes, environmentsHandler.RegisterEnvironmentsRoutes),
	)
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
