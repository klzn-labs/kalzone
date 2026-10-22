package catalog

import (
	"context"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	database "github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/app/observe/entities"
)

type ServiceLister interface {
	List(ctx context.Context, filter catalog.ServiceFilter) ([]catalog.Service, error)
}

type Attachments interface {
	AttachedTo(ctx context.Context, serviceID string) ([]database.Database, error)
}

type Services struct {
	services    ServiceLister
	attachments Attachments
}

func NewServices(services ServiceLister, attachments Attachments) Services {
	return Services{services: services, attachments: attachments}
}

func (s Services) Services(ctx context.Context) ([]entities.Service, error) {
	all, err := s.services.List(ctx, catalog.ServiceFilter{})
	if err != nil {
		return nil, err
	}
	result := make([]entities.Service, 0, len(all))
	for _, svc := range live(all) {
		attached, err := s.attachments.AttachedTo(ctx, svc.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, entities.Service{Slug: svc.Slug, Name: svc.Name, TeamID: svc.OwnerTeamID, Databases: databasesOf(attached)})
	}
	return result, nil
}

func (s Services) Simulated(ctx context.Context) ([]entities.Service, error) {
	all, err := s.services.List(ctx, catalog.ServiceFilter{})
	if err != nil {
		return nil, err
	}
	result := []entities.Service{}
	for _, svc := range all {
		if svc.Simulated && !svc.Decommissioning() {
			result = append(result, entities.Service{Slug: svc.Slug, Name: svc.Name, TeamID: svc.OwnerTeamID})
		}
	}
	return result, nil
}

func databasesOf(attached []database.Database) []entities.Database {
	result := make([]entities.Database, 0, len(attached))
	for _, db := range attached {
		result = append(result, entities.Database{Environment: db.Environment, Schema: db.Schema, User: db.Username, MaxConnections: db.MaxConnections})
	}
	return result
}

func live(all []catalog.Service) []catalog.Service {
	result := make([]catalog.Service, 0, len(all))
	for _, svc := range all {
		if !svc.Decommissioning() && !svc.Simulated {
			result = append(result, svc)
		}
	}
	return result
}
