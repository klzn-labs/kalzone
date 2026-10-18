package envconfig

import (
	"context"

	envconfig "github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/klzn-labs/kalzone/app/onboarding/entities"
)

const devEnvironment = "dev"

type Configs interface {
	Current(ctx context.Context, serviceSlug, environment string) (envconfig.Revision, error)
}

type Source struct {
	configs Configs
}

func NewSource(configs Configs) Source {
	return Source{configs: configs}
}

func (s Source) DevSpec(ctx context.Context, serviceSlug string) (entities.DevSpec, error) {
	rev, err := s.configs.Current(ctx, serviceSlug, devEnvironment)
	if err != nil {
		return entities.DevSpec{}, err
	}
	return entities.DevSpec{Found: true, Port: rev.Spec.Port, HealthPath: rev.Spec.HealthPath}, nil
}
