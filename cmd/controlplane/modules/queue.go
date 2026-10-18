package modules

import (
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/environments"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/services"
	"github.com/klzn-labs/kalzone/app/envconfig/usecases/configs"
	"github.com/klzn-labs/kalzone/app/queue/handlers/gatewayapi"
	queuesHandler "github.com/klzn-labs/kalzone/app/queue/handlers/queues"
	queueCatalog "github.com/klzn-labs/kalzone/app/queue/repositories/catalog"
	queueConfig "github.com/klzn-labs/kalzone/app/queue/repositories/envconfig"
	"github.com/klzn-labs/kalzone/app/queue/repositories/mysql"
	"github.com/klzn-labs/kalzone/app/queue/usecases/credentials"
	"github.com/klzn-labs/kalzone/app/queue/usecases/gateway"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/app/team/usecases/teams"
	"github.com/klzn-labs/kalzone/internal/eventbus"
	"github.com/klzn-labs/kalzone/internal/secretbox"
	"go.uber.org/fx"
)

func Queue() fx.Option {
	return fx.Options(
		fx.Provide(
			mysql.NewQueueRepository, queueCatalog.NewLookup, queueConfig.NewPorts,
			credentials.NewCredentialsUsecase, gateway.NewStatsStore, gateway.NewGatewayUsecase, queues.NewQueuesUsecase,
			queuesHandler.NewQueuesHandler, gatewayapi.NewGatewayAPIHandler,
		),
		queueAdapterInjections(),
		queueUsecaseInjections(),
		fx.Invoke(queuesHandler.RegisterQueuesRoutes, gatewayapi.RegisterGatewayAPIRoutes),
	)
}

func queueAdapterInjections() fx.Option {
	return fx.Provide(
		func(u services.ServicesUsecase) queueCatalog.ServiceFinder { return u },
		func(u environments.EnvironmentsUsecase) queueCatalog.EnvironmentFinder { return u },
		func(u teams.TeamsUsecase) queueCatalog.TeamFinder { return u },
		func(u services.ServicesUsecase) queueConfig.ServiceFinder { return u },
		func(u configs.ConfigUsecase) queueConfig.Configs { return u },
		func(l queueCatalog.Lookup) queuesHandler.ServiceNames { return l },
	)
}

func queueUsecaseInjections() fx.Option {
	return fx.Provide(
		func(r mysql.QueueRepository) queues.Store { return r },
		func(r mysql.QueueRepository) credentials.Store { return r },
		func(r mysql.QueueRepository) gateway.QueueSource { return r },
		func(l queueCatalog.Lookup) queues.ServiceLookup { return l },
		func(l queueCatalog.Lookup) queues.EnvironmentLookup { return l },
		func(l queueCatalog.Lookup) queues.TeamLookup { return l },
		func(l queueCatalog.Lookup) gateway.ServiceLookup { return l },
		func(p queueConfig.Ports) gateway.PortLookup { return p },
		func(s *gateway.StatsStore) queues.StatsReader { return s },
		func(u credentials.CredentialsUsecase) gateway.Credentials { return u },
		func(b *secretbox.Box) credentials.Sealer { return b },
		func(p authz.Policy) queues.Authorizer { return p },
		func(b *eventbus.Bus) queues.Publisher { return b },
		func(u queues.QueuesUsecase) queuesHandler.Queues { return u },
		func(u gateway.GatewayUsecase) gatewayapi.Gateway { return u },
	)
}
