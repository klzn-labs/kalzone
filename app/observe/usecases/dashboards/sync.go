package dashboards

import (
	"context"
	"errors"
	"sort"

	"github.com/klzn-labs/kalzone/app/observe/entities"
)

var syncedOn = map[string]bool{
	"service.created": true, "service.updated": true,
	"database.attached": true, "database.detached": true, "database.adopted": true, "database.deleted": true, "database.released": true,
}

func (u DashboardsUsecase) Sync(ctx context.Context) error {
	services, err := u.catalog.Services(ctx)
	if err != nil {
		return err
	}
	services = WithSharing(services)
	_, writeErr := u.Write(ctx, services)
	return errors.Join(writeErr, u.WritePlatform(ctx), u.publish(ctx, services))
}

func (u DashboardsUsecase) publish(ctx context.Context, services []entities.Service) error {
	if !u.publisher.Enabled() {
		return nil
	}
	return u.publisher.PublishAttachments(ctx, services)
}

func WithSharing(services []entities.Service) []entities.Service {
	users := usersBySchema(services)
	for _, s := range services {
		for i, db := range s.Databases {
			s.Databases[i].SharedWith = othersThan(s.Slug, users[db.Environment+"/"+db.Schema])
		}
	}
	return services
}

func usersBySchema(services []entities.Service) map[string][]string {
	users := map[string][]string{}
	for _, s := range services {
		for _, db := range s.Databases {
			key := db.Environment + "/" + db.Schema
			users[key] = append(users[key], s.Slug)
		}
	}
	return users
}

func othersThan(slug string, slugs []string) []string {
	others := []string{}
	for _, other := range slugs {
		if other != slug {
			others = append(others, other)
		}
	}
	sort.Strings(others)
	return others
}
