package modules

import (
	"context"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/environments"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/services"
	databasesHandler "github.com/klzn-labs/kalzone/app/database/handlers/databases"
	databaseCatalog "github.com/klzn-labs/kalzone/app/database/repositories/catalog"
	databaseDeploy "github.com/klzn-labs/kalzone/app/database/repositories/deploy"
	databaseMysql "github.com/klzn-labs/kalzone/app/database/repositories/mysql"
	"github.com/klzn-labs/kalzone/app/database/repositories/provisioner"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/klzn-labs/kalzone/app/deploy/repositories/mysql"
	"github.com/klzn-labs/kalzone/app/team/usecases/teams"
	"github.com/klzn-labs/kalzone/internal/eventbus"
	"github.com/klzn-labs/kalzone/internal/secretbox"
	"github.com/klzn-labs/kalzone/pkg/config"
	"go.uber.org/fx"
)

func Database() fx.Option {
	return fx.Options(
		fx.Provide(
			databaseMysql.NewDatabaseRepository, provisioner.New, databaseCatalog.NewLookup,
			databaseDeploy.NewRunning, databases.NewDatabasesUsecase, databasesHandler.NewDatabasesHandler, databaseEndpoint,
		),
		databaseAdapterInjections(),
		databaseUsecaseInjections(),
		fx.Invoke(closeProvisioner, databasesHandler.RegisterDatabasesRoutes),
	)
}

func databaseAdapterInjections() fx.Option {
	return fx.Provide(
		func(u services.ServicesUsecase) databaseCatalog.ServiceFinder { return u },
		func(u teams.TeamsUsecase) databaseCatalog.TeamFinder { return u },
		func(u environments.EnvironmentsUsecase) databaseCatalog.ProtectionReader { return u },
		func(r mysql.DeploymentRepository) databaseDeploy.Deployments { return r },
	)
}

func databaseUsecaseInjections() fx.Option {
	return fx.Provide(
		func(r databaseMysql.DatabaseRepository) databases.Store { return r },
		func(p *provisioner.Provisioner) databases.Provisioner { return p },
		func(l databaseCatalog.Lookup) databases.Catalog { return l },
		func(r databaseDeploy.Running) databases.Running { return r },
		func(b *secretbox.Box) databases.Sealer { return b },
		func(p authz.Policy) databases.Authorizer { return p },
		func(b *eventbus.Bus) databases.Publisher { return b },
		func(u databases.DatabasesUsecase) databasesHandler.Databases { return u },
	)
}

func databaseEndpoint(cfg config.Config) databases.Endpoint {
	return databases.Endpoint{Host: cfg.Databases.Host, Port: cfg.Databases.Port}
}

func closeProvisioner(lc fx.Lifecycle, p *provisioner.Provisioner) {
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return p.Close() }})
}
