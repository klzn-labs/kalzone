package modules

import (
	auditHandler "github.com/klzn-labs/kalzone/app/audit/handlers/audit"
	"github.com/klzn-labs/kalzone/app/audit/repositories/lookups"
	"github.com/klzn-labs/kalzone/app/audit/repositories/mysql"
	"github.com/klzn-labs/kalzone/app/audit/usecases/query"
	"github.com/klzn-labs/kalzone/app/audit/usecases/record"
	"github.com/klzn-labs/kalzone/app/catalog/usecases/services"
	"github.com/klzn-labs/kalzone/app/identity/usecases/users"
	"github.com/klzn-labs/kalzone/app/team/usecases/teams"
	"github.com/klzn-labs/kalzone/pkg/events"
	"go.uber.org/fx"
)

func Audit() fx.Option {
	return fx.Options(
		fx.Provide(
			mysql.NewEventRepository, record.NewRecordUsecase, query.NewQueryUsecase,
			lookups.NewResolver, auditHandler.NewAuditHandler,
			fx.Annotate(func(u record.RecordUsecase) events.Subscriber { return u }, fx.ResultTags(`group:"subscribers"`)),
		),
		auditInjections(),
		fx.Invoke(auditHandler.RegisterAuditRoutes),
	)
}

func auditInjections() fx.Option {
	return fx.Provide(
		func(r mysql.EventRepository) record.EventStore { return r },
		func(r mysql.EventRepository) query.EventStore { return r },
		func(u teams.TeamsUsecase) lookups.TeamFinder { return u },
		func(u services.ServicesUsecase) lookups.ServiceFinder { return u },
		func(u users.UsersUsecase) lookups.UserFinder { return u },
		func(u query.QueryUsecase) auditHandler.Events { return u },
		func(r lookups.Resolver) auditHandler.FilterResolver { return r },
	)
}
