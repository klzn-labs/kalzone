package options

import (
	"github.com/adrianozp/platform/cmd/controlplane/modules"
	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/internal/db"
	"github.com/adrianozp/platform/internal/eventbus"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/httpserver/wait"
	"github.com/adrianozp/platform/internal/migrator"
	"github.com/adrianozp/platform/internal/secretbox"
	"github.com/adrianozp/platform/pkg/config"
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
		fx.Invoke(migrator.MigrateOnStart, httpserver.RegisterDocumentRoutes),
	)
}
