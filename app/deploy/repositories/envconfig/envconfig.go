package envconfig

import (
	"context"

	"github.com/adrianozp/platform/app/deploy/entities"
	envconfig "github.com/adrianozp/platform/app/envconfig/entities"
)

type Configs interface {
	CurrentNumber(ctx context.Context, serviceSlug, environment string) (int, error)
	Resolve(ctx context.Context, serviceID, environment string, number int) (envconfig.Resolved, error)
}

type Source struct {
	configs Configs
}

func NewSource(configs Configs) Source {
	return Source{configs: configs}
}

func (s Source) CurrentNumber(ctx context.Context, serviceSlug, environment string) (int, error) {
	return s.configs.CurrentNumber(ctx, serviceSlug, environment)
}

func (s Source) ResolveConfig(ctx context.Context, serviceID, environment string, number int) (entities.ResolvedConfig, error) {
	resolved, err := s.configs.Resolve(ctx, serviceID, environment, number)
	if err != nil {
		return entities.ResolvedConfig{}, err
	}
	spec := resolved.Revision.Spec
	return entities.ResolvedConfig{
		Number: resolved.Revision.Number, Env: resolved.Revision.Env, Secrets: resolved.Secrets,
		Spec: entities.RuntimeSpec{
			Port: spec.Port, HealthPath: spec.HealthPath, Host: spec.Host, Aliases: spec.Aliases, Internal: spec.Internal, MemoryMB: spec.MemoryMB,
			CPUMillis: spec.CPUMillis, Replicas: spec.Replicas, Command: spec.Command,
		},
	}, nil
}
