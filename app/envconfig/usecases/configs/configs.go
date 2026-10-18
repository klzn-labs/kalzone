package configs

import (
	"context"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/events"
)

type Store interface {
	Latest(ctx context.Context, serviceID, environment string) (entities.Revision, bool, error)
	History(ctx context.Context, serviceID, environment string, before, limit int) ([]entities.Revision, error)
	Insert(ctx context.Context, rev entities.Revision) error
	HostOwners(ctx context.Context, hosts []string, serviceID, environment string) ([]string, error)
	InternalOwners(ctx context.Context, serviceID, environment string) ([]string, error)
	ByNumber(ctx context.Context, serviceID, environment string, number int) (entities.Revision, error)
	InsertSecretVersion(ctx context.Context, v entities.SecretVersion) error
	SecretVersions(ctx context.Context, ids []string) ([]entities.SecretVersion, error)
}

type ServiceLookup interface {
	FindService(ctx context.Context, slug string) (entities.ServiceRef, error)
	ServiceExists(ctx context.Context, id string) (bool, error)
}

type EnvironmentLookup interface {
	EnvironmentExists(ctx context.Context, slug string) error
}

type Sealer interface {
	Seal(plaintext, additionalData []byte) (ciphertext, nonce []byte, keyID string, err error)
	Open(ciphertext, nonce, additionalData []byte, keyID string) ([]byte, error)
}

type Authorizer interface {
	Gate(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) (authz.Gate, error)
}

type Requests interface {
	Submit(ctx context.Context, a actor.Actor, c approval.Change) (*approval.Pending, error)
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
	requests       Requests
	allowedDomains []string
}

func NewConfigUsecase(cfg config.Config, store Store, services ServiceLookup, environments EnvironmentLookup, sealer Sealer, authz Authorizer, publisher Publisher, requests Requests) ConfigUsecase {
	return ConfigUsecase{
		store: store, services: services, environments: environments, sealer: sealer,
		authz: authz, publisher: publisher, requests: requests, allowedDomains: cfg.AllowedDomains,
	}
}

type target struct {
	service     entities.ServiceRef
	environment string
}

func (t target) resource() authz.Resource {
	return authz.Resource{TeamID: t.service.OwnerTeamID, Environment: t.environment}
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
	return u.store.History(ctx, t.service.ID, t.environment, 0, clampLimit(limit))
}

func clampLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 100
	}
	return limit
}
