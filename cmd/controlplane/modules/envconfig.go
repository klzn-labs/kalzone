package modules

import (
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/environments"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/services"
	configsHandler "github.com/klzn-labs/kalzone/app/envconfig/handlers/configs"
	searchHandler "github.com/klzn-labs/kalzone/app/envconfig/handlers/search"
	"github.com/klzn-labs/kalzone/app/envconfig/repositories/catalog"
	"github.com/klzn-labs/kalzone/app/envconfig/repositories/mysql"
	"github.com/klzn-labs/kalzone/app/envconfig/usecases/configs"
	"github.com/klzn-labs/kalzone/app/envconfig/usecases/search"
	"github.com/klzn-labs/kalzone/app/team/usecases/teams"
	"github.com/klzn-labs/kalzone/internal/eventbus"
	"github.com/klzn-labs/kalzone/internal/secretbox"
	"go.uber.org/fx"
)

func EnvConfig() fx.Option {
	return fx.Options(
		fx.Provide(
			mysql.NewConfigRepository, catalog.NewLookup, catalog.NewDirectory,
			configs.NewConfigUsecase, search.NewSearchUsecase, configsHandler.NewConfigsHandler, searchHandler.NewSearchHandler,
		),
		envConfigInjections(),
		searchInjections(),
		fx.Invoke(configsHandler.RegisterConfigsRoutes, searchHandler.RegisterSearchRoutes),
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

func searchInjections() fx.Option {
	return fx.Provide(
		func(r mysql.ConfigRepository) search.Store { return r },
		func(u services.ServicesUsecase) catalog.ServiceLister { return u },
		func(u teams.TeamsUsecase) catalog.TeamFinder { return u },
		func(d catalog.Directory) search.Directory { return d },
		func(l catalog.Lookup) search.EnvironmentLookup { return l },
		func(u search.SearchUsecase) searchHandler.Search { return u },
	)
}
