package modules

import (
	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/catalog/usecases/environments"
	"github.com/adrianozp/platform/app/catalog/usecases/services"
	"github.com/adrianozp/platform/app/deploy/handlers/agentapi"
	deploymentsHandler "github.com/adrianozp/platform/app/deploy/handlers/deployments"
	deployCatalog "github.com/adrianozp/platform/app/deploy/repositories/catalog"
	deployConfig "github.com/adrianozp/platform/app/deploy/repositories/envconfig"
	"github.com/adrianozp/platform/app/deploy/repositories/mysql"
	deployRelease "github.com/adrianozp/platform/app/deploy/repositories/release"
	"github.com/adrianozp/platform/app/deploy/usecases/agents"
	"github.com/adrianozp/platform/app/deploy/usecases/deployments"
	"github.com/adrianozp/platform/app/envconfig/usecases/configs"
	"github.com/adrianozp/platform/app/release/usecases/releases"
	"github.com/adrianozp/platform/internal/eventbus"
	"github.com/adrianozp/platform/internal/sweeper"
	"go.uber.org/fx"
)

func Deploy() fx.Option {
	return fx.Options(
		fx.Provide(
			mysql.NewDeploymentRepository, deployCatalog.NewLookup, deployRelease.NewLookup, deployConfig.NewSource,
			deployments.NewDeploymentsUsecase, agents.NewAgentsUsecase,
			deploymentsHandler.NewDeploymentsHandler, agentapi.NewAgentAPIHandler,
		),
		deployAdapterInjections(),
		deployUsecaseInjections(),
		fx.Invoke(deploymentsHandler.RegisterDeploymentsRoutes, agentapi.RegisterAgentAPIRoutes, startDeploySweeper),
	)
}

func deployAdapterInjections() fx.Option {
	return fx.Provide(
		func(u services.ServicesUsecase) deployCatalog.ServiceFinder { return u },
		func(u environments.EnvironmentsUsecase) deployCatalog.EnvironmentFinder { return u },
		func(u releases.ReleasesUsecase) deployRelease.ReleaseFinder { return u },
		func(u configs.ConfigUsecase) deployConfig.Configs { return u },
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
		func(p authz.Policy) deployments.Authorizer { return p },
		func(b *eventbus.Bus) deployments.Publisher { return b },
		func(b *eventbus.Bus) agents.Publisher { return b },
		func(u deployments.DeploymentsUsecase) deploymentsHandler.Deployments { return u },
		func(u agents.AgentsUsecase) agentapi.Agents { return u },
	)
}

func startDeploySweeper(lc fx.Lifecycle, u agents.AgentsUsecase) {
	sweeper.Start(lc, "deploy", u)
}
