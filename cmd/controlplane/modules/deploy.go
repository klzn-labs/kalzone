package modules

import (
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/environments"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/services"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/klzn-labs/kalzone/app/deploy/handlers/agentapi"
	deploymentsHandler "github.com/klzn-labs/kalzone/app/deploy/handlers/deployments"
	runtimeHandler "github.com/klzn-labs/kalzone/app/deploy/handlers/runtime"
	deployCatalog "github.com/klzn-labs/kalzone/app/deploy/repositories/catalog"
	deployChangelog "github.com/klzn-labs/kalzone/app/deploy/repositories/changelog"
	deployDatabase "github.com/klzn-labs/kalzone/app/deploy/repositories/database"
	deployConfig "github.com/klzn-labs/kalzone/app/deploy/repositories/envconfig"
	"github.com/klzn-labs/kalzone/app/deploy/repositories/metrics"
	"github.com/klzn-labs/kalzone/app/deploy/repositories/mysql"
	deployQueue "github.com/klzn-labs/kalzone/app/deploy/repositories/queue"
	deployRelease "github.com/klzn-labs/kalzone/app/deploy/repositories/release"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/agents"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/announce"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/autodeploy"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/deployments"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/runtime"
	"github.com/klzn-labs/kalzone/app/envconfig/usecases/configs"
	"github.com/klzn-labs/kalzone/app/observe/repositories/grafana"
	"github.com/klzn-labs/kalzone/app/queue/usecases/credentials"
	"github.com/klzn-labs/kalzone/app/release/usecases/changelogs"
	"github.com/klzn-labs/kalzone/app/release/usecases/releases"
	"github.com/klzn-labs/kalzone/internal/discord"
	"github.com/klzn-labs/kalzone/internal/eventbus"
	"github.com/klzn-labs/kalzone/internal/sweeper"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/events"
	"go.uber.org/fx"
)

func Deploy() fx.Option {
	return fx.Options(
		fx.Provide(
			mysql.NewDeploymentRepository, deployCatalog.NewLookup, deployRelease.NewLookup, deployConfig.NewSource, deployChangelog.NewShipments,
			deployments.NewDeploymentsUsecase, agents.NewAgentsUsecase,
			deploymentsHandler.NewDeploymentsHandler, agentapi.NewAgentAPIHandler,
			runtime.NewRuntimeUsecase, runtimeHandler.NewRuntimeHandler, autodeploy.NewAutoDeployUsecase,
			announce.NewAnnounceUsecase,
		),
		deployAdapterInjections(),
		deployUsecaseInjections(),
		deployRuntimeInjections(),
		shipmentInjections(),
		fx.Invoke(deploymentsHandler.RegisterDeploymentsRoutes, agentapi.RegisterAgentAPIRoutes, runtimeHandler.RegisterRuntimeRoutes, startDeploySweeper, subscribeAutoDeploy),
	)
}

func deployAdapterInjections() fx.Option {
	return fx.Provide(
		func(u services.ServicesUsecase) deployCatalog.ServiceFinder { return u },
		func(u environments.EnvironmentsUsecase) deployCatalog.EnvironmentFinder { return u },
		func(u releases.ReleasesUsecase) deployRelease.ReleaseFinder { return u },
		func(u configs.ConfigUsecase) deployConfig.Configs { return u },
		func(u credentials.CredentialsUsecase) deployQueue.Credentials { return u },
		func(u databases.DatabasesUsecase) deployDatabase.Databases { return u },
		func(c deployQueue.Credentials, cfg config.Config) deployQueue.Access {
			return deployQueue.NewAccess(c, cfg.Queues.GatewayURL)
		},
		func(c grafana.Client, cfg config.Config) metrics.Metrics {
			return metrics.New(c, cfg.Grafana.PrometheusUID)
		},
	)
}

func deployUsecaseInjections() fx.Option {
	return fx.Provide(
		func(r mysql.DeploymentRepository) deployments.Store { return r },
		func(r mysql.DeploymentRepository) agents.Store { return r },
		func(l deployCatalog.Lookup) deployments.ServiceLookup { return l },
		func(l deployCatalog.Lookup) deployments.EnvironmentLookup { return l },
		func(l deployCatalog.Lookup) agents.ServiceLookup { return l },
		func(l deployRelease.Lookup) deployments.ReleaseLookup { return l },
		func(s deployConfig.Source) deployments.ConfigSource { return s },
		func(s deployConfig.Source) agents.ConfigResolver { return s },
		func(a deployQueue.Access) agents.QueueAccess { return a },
		deployDatabase.NewAccess,
		func(a deployDatabase.Access) agents.DatabaseAccess { return a },
		func(p authz.Policy) deployments.Authorizer { return p },
		func(b *eventbus.Bus) deployments.Publisher { return b },
		func(b *eventbus.Bus) agents.Publisher { return b },
		func(u deployments.DeploymentsUsecase) deploymentsHandler.Deployments { return u },
		func(u agents.AgentsUsecase) agentapi.Agents { return u },
	)
}

func deployRuntimeInjections() fx.Option {
	return fx.Provide(
		func(r mysql.DeploymentRepository) runtime.Store { return r },
		func(l deployCatalog.Lookup) runtime.ServiceLookup { return l },
		func(m metrics.Metrics) runtime.Meter { return m },
		func(u runtime.RuntimeUsecase) runtimeHandler.Runtime { return u },
		func(l deployCatalog.Lookup) autodeploy.ServiceLookup { return l },
		func(u deployments.DeploymentsUsecase) autodeploy.Deployer { return u },
		func(n discord.Notifier) announce.Notifier { return n },
		fx.Annotate(func(u announce.AnnounceUsecase) events.Subscriber { return u }, fx.ResultTags(`group:"subscribers"`)),
	)
}

func subscribeAutoDeploy(b *eventbus.Bus, u autodeploy.AutoDeployUsecase) {
	b.Subscribe(u)
}

func startDeploySweeper(lc fx.Lifecycle, u agents.AgentsUsecase) {
	sweeper.Start(lc, "deploy", u)
}

func shipmentInjections() fx.Option {
	return fx.Provide(
		func(u changelogs.ChangelogsUsecase) deployChangelog.Between { return u },
		func(s deployChangelog.Shipments) deployments.Shipments { return s },
	)
}
