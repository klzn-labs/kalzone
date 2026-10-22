package modules

import (
	"github.com/klzn-labs/kalzone/app/approvals/usecases/requests"
	auditLookups "github.com/klzn-labs/kalzone/app/audit/repositories/lookups"
	"github.com/klzn-labs/kalzone/app/audit/usecases/query"
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/decommission"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/environments"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/services"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/agents"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/deployments"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/platform"
	"github.com/klzn-labs/kalzone/app/envconfig/usecases/configs"
	"github.com/klzn-labs/kalzone/app/identity/usecases/tokens"
	"github.com/klzn-labs/kalzone/app/identity/usecases/users"
	"github.com/klzn-labs/kalzone/app/observe/repositories/grafana"
	"github.com/klzn-labs/kalzone/app/observe/usecases/health"
	"github.com/klzn-labs/kalzone/app/observe/usecases/orgs"
	queueCatalog "github.com/klzn-labs/kalzone/app/queue/repositories/catalog"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/app/release/usecases/changelogs"
	"github.com/klzn-labs/kalzone/app/release/usecases/releases"
	"github.com/klzn-labs/kalzone/app/team/usecases/members"
	"github.com/klzn-labs/kalzone/app/team/usecases/source"
	"github.com/klzn-labs/kalzone/app/team/usecases/teams"
	"github.com/klzn-labs/kalzone/app/web"
	agentsPage "github.com/klzn-labs/kalzone/app/web/handlers/agents"
	auditPage "github.com/klzn-labs/kalzone/app/web/handlers/audit"
	databasesPage "github.com/klzn-labs/kalzone/app/web/handlers/databases"
	environmentPage "github.com/klzn-labs/kalzone/app/web/handlers/environment"
	"github.com/klzn-labs/kalzone/app/web/handlers/login"
	overviewPage "github.com/klzn-labs/kalzone/app/web/handlers/overview"
	peoplePage "github.com/klzn-labs/kalzone/app/web/handlers/people"
	platformPage "github.com/klzn-labs/kalzone/app/web/handlers/platform"
	queuesPage "github.com/klzn-labs/kalzone/app/web/handlers/queues"
	releasesPage "github.com/klzn-labs/kalzone/app/web/handlers/releases"
	requestsPage "github.com/klzn-labs/kalzone/app/web/handlers/requests"
	resourcesPage "github.com/klzn-labs/kalzone/app/web/handlers/resources"
	servicePage "github.com/klzn-labs/kalzone/app/web/handlers/service"
	servicesPage "github.com/klzn-labs/kalzone/app/web/handlers/services"
	teamPage "github.com/klzn-labs/kalzone/app/web/handlers/team"
	teamsPage "github.com/klzn-labs/kalzone/app/web/handlers/teams"
	tokensPage "github.com/klzn-labs/kalzone/app/web/handlers/tokens"
	"go.uber.org/fx"
)

func Web() fx.Option {
	return fx.Options(
		fx.Provide(
			web.NewRenderer, web.NewViews, web.NewStatuses, statusSources,
			login.NewLoginHandler, overviewPage.NewOverviewHandler, servicesPage.NewServicesHandler,
			servicePage.NewServiceHandler, environmentPage.NewEnvironmentHandler, teamsPage.NewTeamsHandler,
			teamPage.NewTeamHandler, auditPage.NewAuditHandler, peoplePage.NewPeopleHandler,
			releasesPage.NewReleasesHandler, agentsPage.NewAgentsHandler, platformPage.NewPlatformHandler, queuesPage.NewQueuesHandler, databasesPage.NewDatabasesHandler, resourcesPage.NewResourcesHandler,
			tokensPage.NewTokensHandler, requestsPage.NewRequestsHandler,
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
		func(u platform.PlatformUsecase) platformPage.Platform { return u },
		func(u platform.PlatformUsecase) servicesPage.Platform { return u },
		func(u queues.QueuesUsecase) queuesPage.Queues { return u },
		func(l queueCatalog.Lookup) queuesPage.ServiceNames { return l },
		func(h queuesPage.QueuesHandler) resourcesPage.QueueSection { return h },
		func(u databases.DatabasesUsecase) databasesPage.Databases { return u },
		func(u services.ServicesUsecase) databasesPage.Services { return u },
		func(u teams.TeamsUsecase) databasesPage.Teams { return u },
		func(h databasesPage.DatabasesHandler) resourcesPage.DatabaseSection { return h },
		func(u environments.EnvironmentsUsecase) web.EnvironmentLister { return u },
		func(u users.UsersUsecase) web.UserFinder { return u },
		func(p authz.Policy) web.Authorizer { return p },
	)
}

func pageInjections() fx.Option {
	return fx.Options(overviewInjections(), catalogPageInjections(), teamPageInjections(), auditPageInjections(), tokenPageInjections())
}

func overviewInjections() fx.Option {
	return fx.Provide(overviewDeps)
}

func overviewDeps(s services.ServicesUsecase, t teams.TeamsUsecase, q query.QueryUsecase, r requests.RequestsUsecase, d deployments.DeploymentsUsecase, a agents.AgentsUsecase) overviewPage.Deps {
	return overviewPage.Deps{Services: s, Teams: t, Events: q, Requests: r, Deployments: d, Agents: a}
}

func catalogPageInjections() fx.Option {
	return fx.Provide(
		func(u services.ServicesUsecase) servicesPage.Services { return u },
		func(u teams.TeamsUsecase) servicesPage.Teams { return u },
		serviceDeps,
		func(u services.ServicesUsecase) environmentPage.Services { return u },
		func(u configs.ConfigUsecase) environmentPage.Configs { return u },
		func(u deployments.DeploymentsUsecase) environmentPage.Deployments { return u },
		func(u releases.ReleasesUsecase) environmentPage.Releases { return u },
		func(u orgs.OrgsUsecase) environmentPage.Grafana { return u },
		func(c grafana.Client) platformPage.Grafana { return c },
		func(u services.ServicesUsecase) releasesPage.Services { return u },
		func(u releases.ReleasesUsecase) releasesPage.Releases { return u },
		func(u changelogs.ChangelogsUsecase) releasesPage.Changelogs { return u },
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
		func(u users.UsersUsecase) teamPage.Users { return u },
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
		func(u requests.RequestsUsecase) requestsPage.Requests { return u },
	)
}

func pageRoutes() fx.Option {
	return fx.Invoke(
		web.RegisterStaticRoutes, login.RegisterLoginRoutes, overviewPage.RegisterOverviewRoutes,
		servicesPage.RegisterServicesRoutes, servicePage.RegisterServiceRoutes, environmentPage.RegisterEnvironmentRoutes,
		teamsPage.RegisterTeamsRoutes, teamPage.RegisterTeamRoutes, auditPage.RegisterAuditRoutes, peoplePage.RegisterPeopleRoutes,
		releasesPage.RegisterReleasesRoutes, agentsPage.RegisterAgentsRoutes, platformPage.RegisterPlatformRoutes, queuesPage.RegisterQueuesRoutes, databasesPage.RegisterDatabasesRoutes, resourcesPage.RegisterResourcesRoutes,
		tokensPage.RegisterTokensRoutes, requestsPage.RegisterRequestsRoutes,
	)
}

func serviceDeps(s services.ServicesUsecase, d decommission.DecommissionUsecase, t teams.TeamsUsecase, c configs.ConfigUsecase, q query.QueryUsecase, qu queues.QueuesUsecase, db databases.DatabasesUsecase, st web.Statuses) servicePage.Deps {
	return servicePage.Deps{Services: s, Decommission: d, Teams: t, Configs: c, Events: q, Queues: qu, Databases: db, Statuses: st}
}

func statusSources(r releases.ReleasesUsecase, d deployments.DeploymentsUsecase, c configs.ConfigUsecase, db databases.DatabasesUsecase, h health.HealthUsecase, q requests.RequestsUsecase) web.StatusSources {
	return web.StatusSources{Releases: r, Deployments: d, Configs: c, Databases: db, Health: h, Requests: q}
}
