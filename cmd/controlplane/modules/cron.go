package modules

import (
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/environments"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/services"
	cronsHandler "github.com/klzn-labs/kalzone/app/cron/handlers/crons"
	"github.com/klzn-labs/kalzone/app/cron/handlers/queuedfeed"
	cronCatalog "github.com/klzn-labs/kalzone/app/cron/repositories/catalog"
	cronMysql "github.com/klzn-labs/kalzone/app/cron/repositories/mysql"
	"github.com/klzn-labs/kalzone/app/cron/repositories/signing"
	"github.com/klzn-labs/kalzone/app/cron/usecases/crons"
	"github.com/klzn-labs/kalzone/app/cron/usecases/executor"
	queueConfig "github.com/klzn-labs/kalzone/app/queue/repositories/envconfig"
	"github.com/klzn-labs/kalzone/app/queue/usecases/credentials"
	"github.com/klzn-labs/kalzone/app/team/usecases/teams"
	"github.com/klzn-labs/kalzone/internal/eventbus"
	"go.uber.org/fx"
)

func Cron() fx.Option {
	return fx.Options(
		fx.Provide(
			cronMysql.NewCronRepository, cronCatalog.NewLookup, signing.NewKeys,
			crons.NewCronsUsecase, executor.NewExecutorUsecase, cronsHandler.NewCronsHandler, queuedfeed.NewFeed,
		),
		cronAdapterInjections(),
		cronUsecaseInjections(),
		fx.Invoke(cronsHandler.RegisterCronsRoutes),
	)
}

func cronAdapterInjections() fx.Option {
	return fx.Provide(
		func(u services.ServicesUsecase) cronCatalog.ServiceFinder { return u },
		func(u environments.EnvironmentsUsecase) cronCatalog.EnvironmentFinder { return u },
		func(u teams.TeamsUsecase) cronCatalog.TeamFinder { return u },
		func(u credentials.CredentialsUsecase) signing.Credentials { return u },
		func(l cronCatalog.Lookup) cronsHandler.ServiceNames { return l },
	)
}

func cronUsecaseInjections() fx.Option {
	return fx.Provide(
		func(r cronMysql.CronRepository) crons.Store { return r },
		func(r cronMysql.CronRepository) executor.Source { return r },
		func(l cronCatalog.Lookup) crons.Catalog { return l },
		func(l cronCatalog.Lookup) executor.ServiceLookup { return l },
		func(p queueConfig.Ports) executor.PortLookup { return p },
		func(k signing.Keys) executor.Credentials { return k },
		func(p authz.Policy) crons.Authorizer { return p },
		func(b *eventbus.Bus) crons.Publisher { return b },
		func(u crons.CronsUsecase) cronsHandler.Crons { return u },
		func(u executor.ExecutorUsecase) queuedfeed.Executor { return u },
	)
}
