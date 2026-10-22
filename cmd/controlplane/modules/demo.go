package modules

import (
	"context"
	"log"

	approvalsMysql "github.com/klzn-labs/kalzone/app/approvals/repositories/mysql"
	auditMysql "github.com/klzn-labs/kalzone/app/audit/repositories/mysql"
	catalogMysql "github.com/klzn-labs/kalzone/app/catalog/repositories/mysql"
	demoMysql "github.com/klzn-labs/kalzone/app/demo/repositories/mysql"
	"github.com/klzn-labs/kalzone/app/demo/usecases/seed"
	deployMysql "github.com/klzn-labs/kalzone/app/deploy/repositories/mysql"
	envconfigMysql "github.com/klzn-labs/kalzone/app/envconfig/repositories/mysql"
	identityMysql "github.com/klzn-labs/kalzone/app/identity/repositories/mysql"
	releaseMysql "github.com/klzn-labs/kalzone/app/release/repositories/mysql"
	teamMysql "github.com/klzn-labs/kalzone/app/team/repositories/mysql"
	"github.com/klzn-labs/kalzone/internal/sweeper"
	"go.uber.org/fx"
)

func Demo() fx.Option {
	return fx.Options(
		fx.Provide(seed.NewSeedUsecase, demoStore, demoMysql.NewShiftRepository),
		fx.Invoke(seedOnStart, startDemoSweeper),
	)
}

type demoRepositories struct {
	fx.In
	Teams       teamMysql.TeamRepository
	Users       identityMysql.UserRepository
	Services    catalogMysql.ServiceRepository
	Configs     envconfigMysql.ConfigRepository
	Releases    releaseMysql.ReleaseRepository
	Changelogs  releaseMysql.ChangelogRepository
	Deployments deployMysql.DeploymentRepository
	Requests    approvalsMysql.RequestRepository
	Events      auditMysql.EventRepository
	Shifter     demoMysql.ShiftRepository
}

func demoStore(r demoRepositories) seed.Store {
	return seed.Store{
		Teams: r.Teams, Users: r.Users, Services: r.Services, Configs: r.Configs, Releases: r.Releases,
		Changelogs: r.Changelogs, Deployments: r.Deployments, Requests: r.Requests, Events: r.Events, Shifter: r.Shifter,
	}
}

func seedOnStart(lc fx.Lifecycle, u seed.SeedUsecase) {
	lc.Append(fx.Hook{OnStart: func(ctx context.Context) error {
		if err := u.Seed(ctx); err != nil {
			log.Printf("demo seed: %v", err)
		}
		return nil
	}})
}

func startDemoSweeper(lc fx.Lifecycle, u seed.SeedUsecase) {
	sweeper.Start(lc, "demo", u)
}
