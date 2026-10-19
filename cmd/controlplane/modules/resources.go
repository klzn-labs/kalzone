package modules

import (
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	queueCatalog "github.com/klzn-labs/kalzone/app/queue/repositories/catalog"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/app/resources/entities"
	resourcesHandler "github.com/klzn-labs/kalzone/app/resources/handlers/resources"
	resourcesDatabase "github.com/klzn-labs/kalzone/app/resources/repositories/database"
	resourcesQueue "github.com/klzn-labs/kalzone/app/resources/repositories/queue"
	"github.com/klzn-labs/kalzone/app/resources/usecases/resources"
	"go.uber.org/fx"
)

func Resources() fx.Option {
	return fx.Options(
		fx.Provide(
			resourcesQueue.NewSource, resourcesDatabase.NewSource, resources.NewResourcesUsecase, resourcesHandler.NewResourcesHandler,
			func(q resourcesQueue.Source, d resourcesDatabase.Source) resources.Sources {
				return resources.Sources{entities.KindQueue: q, entities.KindDatabase: d}
			},
			func(u databases.DatabasesUsecase) resourcesDatabase.Databases { return u },
			func(u queues.QueuesUsecase) resourcesQueue.Queues { return u },
			func(l queueCatalog.Lookup) resourcesQueue.ServiceNames { return l },
			func(u resources.ResourcesUsecase) resourcesHandler.Resources { return u },
		),
		fx.Invoke(resourcesHandler.RegisterResourcesRoutes),
	)
}
