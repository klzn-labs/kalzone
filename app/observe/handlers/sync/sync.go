package sync

import (
	"context"
	"log"
	"time"

	catalog "github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/app/observe/entities"
	"go.uber.org/fx"
)

type Writer interface {
	Write(ctx context.Context, services []entities.Service) (int, error)
}

type Services interface {
	List(ctx context.Context, filter catalog.ServiceFilter) ([]catalog.Service, error)
}

func OnStart(lc fx.Lifecycle, writer Writer, services Services) {
	lc.Append(fx.StartHook(func() {
		go syncAll(writer, services)
	}))
}

func syncAll(writer Writer, services Services) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	all, err := services.List(ctx, catalog.ServiceFilter{})
	if err != nil {
		log.Printf("dashboards: %v", err)
		return
	}
	list := make([]entities.Service, 0, len(all))
	for _, s := range all {
		list = append(list, entities.Service{Slug: s.Slug, Name: s.Name})
	}
	if written, err := writer.Write(ctx, list); err != nil || written > 0 {
		log.Printf("dashboards: wrote %d, error: %v", written, err)
	}
}
