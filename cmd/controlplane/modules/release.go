package modules

import (
	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/services"
	"github.com/klzn-labs/kalzone/app/release/handlers/callbacks"
	releasesHandler "github.com/klzn-labs/kalzone/app/release/handlers/releases"
	"github.com/klzn-labs/kalzone/app/release/handlers/webhooks"
	"github.com/klzn-labs/kalzone/app/release/repositories/catalog"
	"github.com/klzn-labs/kalzone/app/release/repositories/githubapp"
	"github.com/klzn-labs/kalzone/app/release/repositories/mysql"
	"github.com/klzn-labs/kalzone/app/release/repositories/registry"
	"github.com/klzn-labs/kalzone/app/release/usecases/changelogs"
	"github.com/klzn-labs/kalzone/app/release/usecases/releases"
	"github.com/klzn-labs/kalzone/internal/eventbus"
	"github.com/klzn-labs/kalzone/internal/oidc"
	"github.com/klzn-labs/kalzone/internal/sweeper"
	"go.uber.org/fx"
)

func Release() fx.Option {
	return fx.Options(
		fx.Provide(
			mysql.NewReleaseRepository, catalog.NewLookup, githubapp.NewClient, registry.NewResolver, oidc.NewVerifier,
			releases.NewReleasesUsecase, mysql.NewChangelogRepository, changelogs.NewChangelogsUsecase,
			releasesHandler.NewReleasesHandler, callbacks.NewCallbacksHandler, webhooks.NewWebhooksHandler,
		),
		releaseInjections(),
		changelogInjections(),
		fx.Invoke(
			releasesHandler.RegisterReleasesRoutes, callbacks.RegisterCallbacksRoutes,
			webhooks.RegisterWebhooksRoutes, startReleaseSweeper, startChangelogSweeper, subscribeChangelogs,
		),
	)
}

func releaseInjections() fx.Option {
	return fx.Provide(
		func(r mysql.ReleaseRepository) releases.Store { return r },
		func(u services.ServicesUsecase) catalog.ServiceFinder { return u },
		func(l catalog.Lookup) releases.ServiceLookup { return l },
		func(c githubapp.Client) releases.Builder { return c },
		func(r registry.Resolver) releases.DigestResolver { return r },
		func(p authz.Policy) releases.Authorizer { return p },
		func(b *eventbus.Bus) releases.Publisher { return b },
		func(u releases.ReleasesUsecase) releasesHandler.Releases { return u },
		func(v *oidc.Verifier) callbacks.TokenVerifier { return v },
		func(u releases.ReleasesUsecase) callbacks.Builds { return u },
		func(u releases.ReleasesUsecase) webhooks.Builds { return u },
	)
}

func changelogInjections() fx.Option {
	return fx.Provide(
		func(r mysql.ChangelogRepository) changelogs.Store { return r },
		func(r mysql.ReleaseRepository) changelogs.Releases { return r },
		func(l catalog.Lookup) changelogs.Services { return l },
		func(c githubapp.Client) changelogs.GitHub { return c },
	)
}

func startReleaseSweeper(lc fx.Lifecycle, u releases.ReleasesUsecase) {
	sweeper.Start(lc, "release", u)
}

func startChangelogSweeper(lc fx.Lifecycle, u changelogs.ChangelogsUsecase) {
	sweeper.Start(lc, "changelog", u)
}

func subscribeChangelogs(b *eventbus.Bus, u changelogs.ChangelogsUsecase) {
	b.Subscribe(u)
}
