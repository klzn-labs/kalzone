package releases

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/events"
)

type Store interface {
	Create(ctx context.Context, rel entities.Release) (entities.Release, error)
	Transition(ctx context.Context, rel entities.Release, from entities.Status) error
	FindByID(ctx context.Context, id string) (entities.Release, error)
	ListByService(ctx context.Context, serviceID string, before, limit int) ([]entities.Release, error)
	Unfinished(ctx context.Context, createdBefore time.Time) ([]entities.Release, error)
	UnfinishedFor(ctx context.Context, serviceID string) ([]entities.Release, error)
	LatestBuilt(ctx context.Context, serviceID string) (entities.Release, bool, error)
	ByNumber(ctx context.Context, serviceID string, number int) (entities.Release, bool, error)
	BuiltByCommitPrefix(ctx context.Context, serviceID, prefix string, limit int) ([]entities.Release, error)
	BuiltByTag(ctx context.Context, serviceID, tag string) (entities.Release, bool, error)
}

type ServiceLookup interface {
	FindService(ctx context.Context, slug string) (entities.ServiceRef, error)
	FindServiceByID(ctx context.Context, id string) (entities.ServiceRef, error)
	ServicesForRepository(ctx context.Context, repository string) ([]entities.ServiceRef, error)
}

type Builder interface {
	Dispatch(ctx context.Context, request entities.BuildRequest) error
}

type DigestResolver interface {
	Resolve(ctx context.Context, image, tag string) (string, error)
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

var errNotYours = apperror.Forbidden("this build belongs to another repository")

type ReleasesUsecase struct {
	store         Store
	services      ServiceLookup
	builder       Builder
	resolver      DigestResolver
	authz         Authorizer
	publisher     Publisher
	platformURL   string
	timeout       time.Duration
	buildWorkflow string
	registry      config.Registry
}

func NewReleasesUsecase(cfg config.Config, store Store, services ServiceLookup, builder Builder, resolver DigestResolver, authz Authorizer, publisher Publisher) ReleasesUsecase {
	return ReleasesUsecase{
		store: store, services: services, builder: builder, resolver: resolver, authz: authz,
		publisher: publisher, platformURL: cfg.BaseURL, timeout: cfg.Build.Timeout,
		buildWorkflow: cfg.Build.Workflow, registry: cfg.Registry,
	}
}

func (u ReleasesUsecase) List(ctx context.Context, serviceSlug string, limit int) ([]entities.Release, error) {
	service, err := u.services.FindService(ctx, serviceSlug)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return u.store.ListByService(ctx, service.ID, 0, limit)
}

func (u ReleasesUsecase) Get(ctx context.Context, id string) (entities.Release, error) {
	return u.store.FindByID(ctx, id)
}

func (u ReleasesUsecase) publish(ctx context.Context, a actor.Actor, action string, service entities.ServiceRef, rel entities.Release) {
	details := map[string]any{
		"release": rel.Number, "release_id": rel.ID, "status": string(rel.Status), "trigger": string(rel.Trigger),
	}
	if rel.GitRef != "" {
		details["ref"] = rel.GitRef
	}
	if rel.Failure != "" {
		details["failure"] = rel.Failure
	}
	u.publisher.Publish(ctx, events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action, TargetKind: events.TargetRelease,
		TargetID: service.ID, TargetName: service.Slug, TeamID: service.OwnerTeamID, Details: details,
	})
}
