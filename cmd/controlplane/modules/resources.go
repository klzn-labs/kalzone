package modules

import (
	cronCatalog "github.com/klzn-labs/kalzone/app/cron/repositories/catalog"
	"github.com/klzn-labs/kalzone/app/cron/usecases/crons"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	queueCatalog "github.com/klzn-labs/kalzone/app/queue/repositories/catalog"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/app/resources/entities"
	resourcesHandler "github.com/klzn-labs/kalzone/app/resources/handlers/resources"
	resourcesCron "github.com/klzn-labs/kalzone/app/resources/repositories/cron"
	resourcesDatabase "github.com/klzn-labs/kalzone/app/resources/repositories/database"
	resourcesQueue "github.com/klzn-labs/kalzone/app/resources/repositories/queue"
	"github.com/klzn-labs/kalzone/app/resources/usecases/resources"
	"go.uber.org/fx"
)

func Resources() fx.Option {
	return fx.Options(
		fx.Provide(
			resourcesQueue.NewSource, resourcesDatabase.NewSource, resourcesCron.NewSource, resources.NewResourcesUsecase, resourcesHandler.NewResourcesHandler,
			func(q resourcesQueue.Source, d resourcesDatabase.Source, c resourcesCron.Source) resources.Sources {
				return resources.Sources{entities.KindQueue: q, entities.KindDatabase: d, entities.KindCron: c}
			},
			func(u crons.CronsUsecase) resourcesCron.Crons { return u },
			func(l cronCatalog.Lookup) resourcesCron.ServiceNames { return l },
			func(u databases.DatabasesUsecase) resourcesDatabase.Databases { return u },
			func(u queues.QueuesUsecase) resourcesQueue.Queues { return u },
			func(l queueCatalog.Lookup) resourcesQueue.ServiceNames { return l },
			func(u resources.ResourcesUsecase) resourcesHandler.Resources { return u },
		),
		fx.Invoke(resourcesHandler.RegisterResourcesRoutes),
	)
}
