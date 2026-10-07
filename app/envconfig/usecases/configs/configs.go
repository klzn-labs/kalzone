package configs

import (
	"context"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/adrianozp/platform/pkg/events"
)

type Store interface {
	Latest(ctx context.Context, serviceID, environment string) (entities.Revision, bool, error)
	History(ctx context.Context, serviceID, environment string, limit int) ([]entities.Revision, error)
	Insert(ctx context.Context, rev entities.Revision) error
	HostTaken(ctx context.Context, host, serviceID, environment string) (bool, error)
	InsertSecretVersion(ctx context.Context, v entities.SecretVersion) error
	SecretVersions(ctx context.Context, ids []string) ([]entities.SecretVersion, error)
}

type ServiceLookup interface {
	FindService(ctx context.Context, slug string) (entities.ServiceRef, error)
}

type EnvironmentLookup interface {
	EnvironmentExists(ctx context.Context, slug string) error
}

type Sealer interface {
	Seal(plaintext, additionalData []byte) (ciphertext, nonce []byte, keyID string, err error)
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

type ConfigUsecase struct {
	store          Store
	services       ServiceLookup
	environments   EnvironmentLookup
	sealer         Sealer
	authz          Authorizer
	publisher      Publisher
	allowedDomains []string
}

func NewConfigUsecase(cfg config.Config, store Store, services ServiceLookup, environments EnvironmentLookup, sealer Sealer, authz Authorizer, publisher Publisher) ConfigUsecase {
	return ConfigUsecase{
		store: store, services: services, environments: environments, sealer: sealer,
		authz: authz, publisher: publisher, allowedDomains: cfg.AllowedDomains,
	}
}

type target struct {
	service     entities.ServiceRef
	environment string
}

func (u ConfigUsecase) resolve(ctx context.Context, serviceSlug, environment string) (target, error) {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return target{}, err
	}
	if err := u.environments.EnvironmentExists(ctx, environment); err != nil {
		return target{}, err
	}
	return target{service: service, environment: environment}, nil
}

func (u ConfigUsecase) Current(ctx context.Context, serviceSlug, environment string) (entities.Revision, error) {
	t, err := u.resolve(ctx, serviceSlug, environment)
	if err != nil {
		return entities.Revision{}, err
	}
	return u.current(ctx, t)
}

func (u ConfigUsecase) current(ctx context.Context, t target) (entities.Revision, error) {
	rev, found, err := u.store.Latest(ctx, t.service.ID, t.environment)
	if err != nil || found {
		return rev, err
	}
	return entities.Initial(t.service.ID, t.environment), nil
}

func (u ConfigUsecase) History(ctx context.Context, serviceSlug, environment string, limit int) ([]entities.Revision, error) {
	t, err := u.resolve(ctx, serviceSlug, environment)
	if err != nil {
		return nil, err
	}
	return u.store.History(ctx, t.service.ID, t.environment, clampLimit(limit))
}

func clampLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 100
	}
	return limit
}
