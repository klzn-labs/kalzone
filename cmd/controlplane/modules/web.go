package modules

import (
	auditLookups "github.com/adrianozp/platform/app/audit/repositories/lookups"
	"github.com/adrianozp/platform/app/audit/usecases/query"
	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/catalog/usecases/environments"
	"github.com/adrianozp/platform/app/catalog/usecases/services"
	"github.com/adrianozp/platform/app/deploy/usecases/agents"
	"github.com/adrianozp/platform/app/deploy/usecases/deployments"
	"github.com/adrianozp/platform/app/envconfig/usecases/configs"
	"github.com/adrianozp/platform/app/identity/usecases/tokens"
	"github.com/adrianozp/platform/app/identity/usecases/users"
	"github.com/adrianozp/platform/app/observe/repositories/grafana"
	queueCatalog "github.com/adrianozp/platform/app/queue/repositories/catalog"
	"github.com/adrianozp/platform/app/queue/usecases/queues"
	"github.com/adrianozp/platform/app/release/usecases/releases"
	"github.com/adrianozp/platform/app/team/usecases/members"
	"github.com/adrianozp/platform/app/team/usecases/source"
	"github.com/adrianozp/platform/app/team/usecases/teams"
	"github.com/adrianozp/platform/app/web"
	agentsPage "github.com/adrianozp/platform/app/web/handlers/agents"
	auditPage "github.com/adrianozp/platform/app/web/handlers/audit"
	environmentPage "github.com/adrianozp/platform/app/web/handlers/environment"
	"github.com/adrianozp/platform/app/web/handlers/login"
	overviewPage "github.com/adrianozp/platform/app/web/handlers/overview"
	peoplePage "github.com/adrianozp/platform/app/web/handlers/people"
	queuesPage "github.com/adrianozp/platform/app/web/handlers/queues"
	releasesPage "github.com/adrianozp/platform/app/web/handlers/releases"
	servicePage "github.com/adrianozp/platform/app/web/handlers/service"
	servicesPage "github.com/adrianozp/platform/app/web/handlers/services"
	teamPage "github.com/adrianozp/platform/app/web/handlers/team"
	teamsPage "github.com/adrianozp/platform/app/web/handlers/teams"
	tokensPage "github.com/adrianozp/platform/app/web/handlers/tokens"
	"go.uber.org/fx"
)

func Web() fx.Option {
	return fx.Options(
		fx.Provide(
			web.NewRenderer, web.NewViews,
			login.NewLoginHandler, overviewPage.NewOverviewHandler, servicesPage.NewServicesHandler,
			servicePage.NewServiceHandler, environmentPage.NewEnvironmentHandler, teamsPage.NewTeamsHandler,
			teamPage.NewTeamHandler, auditPage.NewAuditHandler, peoplePage.NewPeopleHandler,
			releasesPage.NewReleasesHandler, agentsPage.NewAgentsHandler, queuesPage.NewQueuesHandler,
			tokensPage.NewTokensHandler,
		),
		viewInjections(),
		pageInjections(),
		pageRoutes(),
	)
}

func viewInjections() fx.Option {
	return fx.Provide(
		func(u teams.TeamsUsecase) web.TeamFinder { return u },
		func(s source.LocalTeamSource) web.MembershipSource { return s },
		func(u configs.ConfigUsecase) web.ConfigReader { return u },
		func(u deployments.DeploymentsUsecase) web.LiveReader { return u },
		func(u agents.AgentsUsecase) agentsPage.Agents { return u },
		func(u queues.QueuesUsecase) queuesPage.Queues { return u },
		func(l queueCatalog.Lookup) queuesPage.ServiceNames { return l },
		func(u environments.EnvironmentsUsecase) web.EnvironmentLister { return u },
		func(u users.UsersUsecase) web.UserFinder { return u },
		func(p authz.Policy) web.Authorizer { return p },
	)
}

func pageInjections() fx.Option {
	return fx.Options(overviewInjections(), catalogPageInjections(), teamPageInjections(), auditPageInjections(), tokenPageInjections())
}

func overviewInjections() fx.Option {
	return fx.Provide(
		func(u services.ServicesUsecase) overviewPage.Services { return u },
		func(u teams.TeamsUsecase) overviewPage.Teams { return u },
		func(u query.QueryUsecase) overviewPage.Events { return u },
	)
}

func catalogPageInjections() fx.Option {
	return fx.Provide(
		func(u services.ServicesUsecase) servicesPage.Services { return u },
		func(u teams.TeamsUsecase) servicesPage.Teams { return u },
		func(u services.ServicesUsecase) servicePage.Services { return u },
		func(u teams.TeamsUsecase) servicePage.Teams { return u },
		func(u configs.ConfigUsecase) servicePage.Configs { return u },
		func(u query.QueryUsecase) servicePage.Events { return u },
		func(u services.ServicesUsecase) environmentPage.Services { return u },
		func(u configs.ConfigUsecase) environmentPage.Configs { return u },
		func(u deployments.DeploymentsUsecase) environmentPage.Deployments { return u },
		func(u releases.ReleasesUsecase) environmentPage.Releases { return u },
		func(c grafana.Client) environmentPage.Grafana { return c },
		func(u services.ServicesUsecase) releasesPage.Services { return u },
		func(u releases.ReleasesUsecase) releasesPage.Releases { return u },
	)
}

func teamPageInjections() fx.Option {
	return fx.Provide(
		func(u teams.TeamsUsecase) teamsPage.Teams { return u },
		func(u members.MembersUsecase) teamsPage.Members { return u },
		func(u services.ServicesUsecase) teamsPage.Services { return u },
		func(u teams.TeamsUsecase) teamPage.Teams { return u },
		func(u members.MembersUsecase) teamPage.Members { return u },
		func(u services.ServicesUsecase) teamPage.Services { return u },
		func(u users.UsersUsecase) peoplePage.Users { return u },
	)
}

func auditPageInjections() fx.Option {
	return fx.Provide(
		func(u query.QueryUsecase) auditPage.Events { return u },
		func(r auditLookups.Resolver) auditPage.FilterResolver { return r },
		func(u teams.TeamsUsecase) auditPage.Teams { return u },
		func(u services.ServicesUsecase) auditPage.Services { return u },
	)
}

func tokenPageInjections() fx.Option {
	return fx.Provide(
		func(u tokens.TokensUsecase) tokensPage.Tokens { return u },
		func(u teams.TeamsUsecase) tokensPage.AllTeams { return u },
	)
}

func pageRoutes() fx.Option {
	return fx.Invoke(
		web.RegisterStaticRoutes, login.RegisterLoginRoutes, overviewPage.RegisterOverviewRoutes,
		servicesPage.RegisterServicesRoutes, servicePage.RegisterServiceRoutes, environmentPage.RegisterEnvironmentRoutes,
		teamsPage.RegisterTeamsRoutes, teamPage.RegisterTeamRoutes, auditPage.RegisterAuditRoutes, peoplePage.RegisterPeopleRoutes,
		releasesPage.RegisterReleasesRoutes, agentsPage.RegisterAgentsRoutes, queuesPage.RegisterQueuesRoutes,
		tokensPage.RegisterTokensRoutes,
	)
}
