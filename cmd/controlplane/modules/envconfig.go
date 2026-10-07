package modules

import (
	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/catalog/usecases/environments"
	"github.com/adrianozp/platform/app/catalog/usecases/services"
	configsHandler "github.com/adrianozp/platform/app/envconfig/handlers/configs"
	"github.com/adrianozp/platform/app/envconfig/repositories/catalog"
	"github.com/adrianozp/platform/app/envconfig/repositories/mysql"
	"github.com/adrianozp/platform/app/envconfig/usecases/configs"
	"github.com/adrianozp/platform/internal/eventbus"
	"github.com/adrianozp/platform/internal/secretbox"
	"go.uber.org/fx"
)

func EnvConfig() fx.Option {
	return fx.Options(
		fx.Provide(mysql.NewConfigRepository, catalog.NewLookup, configs.NewConfigUsecase, configsHandler.NewConfigsHandler),
		envConfigInjections(),
		fx.Invoke(configsHandler.RegisterConfigsRoutes),
	)
}

func envConfigInjections() fx.Option {
	return fx.Provide(
		func(r mysql.ConfigRepository) configs.Store { return r },
		func(u services.ServicesUsecase) catalog.ServiceFinder { return u },
		func(u environments.EnvironmentsUsecase) catalog.EnvironmentFinder { return u },
		func(l catalog.Lookup) configs.ServiceLookup { return l },
		func(l catalog.Lookup) configs.EnvironmentLookup { return l },
		func(b *secretbox.Box) configs.Sealer { return b },
		func(p authz.Policy) configs.Authorizer { return p },
		func(b *eventbus.Bus) configs.Publisher { return b },
		func(u configs.ConfigUsecase) configsHandler.Configs { return u },
	)
}
