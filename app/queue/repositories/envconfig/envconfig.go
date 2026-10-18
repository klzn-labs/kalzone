package envconfig

import (
	"context"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	envconfig "github.com/klzn-labs/kalzone/app/envconfig/entities"
)

type ServiceFinder interface {
	GetByID(ctx context.Context, id string) (catalog.Service, error)
}

type Configs interface {
	Current(ctx context.Context, serviceSlug, environment string) (envconfig.Revision, error)
}

type Ports struct {
	services ServiceFinder
	configs  Configs
}

func NewPorts(services ServiceFinder, configs Configs) Ports {
	return Ports{services: services, configs: configs}
}

func (p Ports) Port(ctx context.Context, serviceID, environment string) (int, error) {
	service, err := p.services.GetByID(ctx, serviceID)
	if err != nil {
		return 0, err
	}
	rev, err := p.configs.Current(ctx, service.Slug, environment)
	return rev.Spec.Port, err
}
