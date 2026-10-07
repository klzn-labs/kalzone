package modules

import (
	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/catalog/usecases/services"
	"github.com/adrianozp/platform/app/release/handlers/callbacks"
	releasesHandler "github.com/adrianozp/platform/app/release/handlers/releases"
	"github.com/adrianozp/platform/app/release/handlers/webhooks"
	"github.com/adrianozp/platform/app/release/repositories/catalog"
	"github.com/adrianozp/platform/app/release/repositories/githubapp"
	"github.com/adrianozp/platform/app/release/repositories/mysql"
	"github.com/adrianozp/platform/app/release/repositories/registry"
	"github.com/adrianozp/platform/app/release/usecases/releases"
	"github.com/adrianozp/platform/internal/eventbus"
	"github.com/adrianozp/platform/internal/oidc"
	"github.com/adrianozp/platform/internal/sweeper"
	"go.uber.org/fx"
)

func Release() fx.Option {
	return fx.Options(
		fx.Provide(
			mysql.NewReleaseRepository, catalog.NewLookup, githubapp.NewClient, registry.NewResolver, oidc.NewVerifier,
			releases.NewReleasesUsecase,
			releasesHandler.NewReleasesHandler, callbacks.NewCallbacksHandler, webhooks.NewWebhooksHandler,
		),
		releaseInjections(),
		fx.Invoke(
			releasesHandler.RegisterReleasesRoutes, callbacks.RegisterCallbacksRoutes,
			webhooks.RegisterWebhooksRoutes, startReleaseSweeper,
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

func startReleaseSweeper(lc fx.Lifecycle, u releases.ReleasesUsecase) {
	sweeper.Start(lc, "release", u)
}
