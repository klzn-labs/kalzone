package options

import (
	"github.com/klzn-labs/kalzone/cmd/controlplane/modules"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/db"
	"github.com/klzn-labs/kalzone/internal/eventbus"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/httpserver/wait"
	"github.com/klzn-labs/kalzone/internal/mcpserver"
	"github.com/klzn-labs/kalzone/internal/migrator"
	"github.com/klzn-labs/kalzone/internal/secretbox"
	"github.com/klzn-labs/kalzone/pkg/config"
	"go.uber.org/fx"
)

func Options() fx.Option {
	return fx.Options(
		Infrastructure(),
		modules.Authz(),
		modules.Identity(),
		modules.Team(),
		modules.Catalog(),
		modules.EnvConfig(),
		modules.Audit(),
		modules.Release(),
		modules.Queue(),
		modules.Deploy(),
		modules.Approvals(),
		modules.Observe(),
		modules.Onboarding(),
		modules.Web(),
	)
}

func Infrastructure() fx.Option {
	return fx.Options(
		fx.Provide(
			config.LoadConfig,
			migrator.New,
			db.NewGormDB,
			httpserver.NewEngine,
			apispec.NewCatalog,
			httpserver.NewRoutes,
			wait.NewDrain,
			wait.NewWaiter,
			eventbus.New,
			secretbox.New,
		),
		fx.Invoke(migrator.MigrateOnStart, httpserver.RegisterDocumentRoutes, mcpserver.RegisterMCPRoutes),
	)
}
