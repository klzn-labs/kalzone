package crons

import (
	"context"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/events"
)

type ServiceRef struct {
	ID              string
	Slug            string
	OwnerTeamID     string
	Decommissioning bool
}

type Store interface {
	Create(ctx context.Context, c entities.Cron) error
	Save(ctx context.Context, c entities.Cron) error
	SetPaused(ctx context.Context, id string, paused bool) error
	Get(ctx context.Context, environment, name string) (entities.Cron, error)
	List(ctx context.Context, environment, teamID string) ([]entities.Cron, error)
	ForService(ctx context.Context, serviceID string) ([]entities.Cron, error)
	Delete(ctx context.Context, id string) error
	AddCommand(ctx context.Context, c entities.Command) error
	Runs(ctx context.Context, cronID string, limit int) ([]entities.Run, error)
}

type Catalog interface {
	FindService(ctx context.Context, slug string) (ServiceRef, error)
	FindServiceByID(ctx context.Context, id string) (ServiceRef, error)
	EnvironmentExists(ctx context.Context, slug string) error
	FindTeam(ctx context.Context, slug string) (string, error)
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

type CronsUsecase struct {
	store     Store
	catalog   Catalog
	authz     Authorizer
	publisher Publisher
	requests  Requests
}

func NewCronsUsecase(store Store, catalog Catalog, authz Authorizer, publisher Publisher, requests Requests) CronsUsecase {
	return CronsUsecase{store: store, catalog: catalog, authz: authz, publisher: publisher, requests: requests}
}

func (u CronsUsecase) editable(ctx context.Context, a actor.Actor, environment, name string) (entities.Cron, authz.Gate, error) {
	c, err := u.store.Get(ctx, environment, name)
	if err != nil {
		return entities.Cron{}, "", err
	}
	gate, err := u.authz.Gate(ctx, a, authz.EditCron, resourceOf(c))
	return c, gate, err
}

func (u CronsUsecase) operable(ctx context.Context, a actor.Actor, environment, name string) (entities.Cron, authz.Gate, error) {
	c, gate, err := u.editable(ctx, a, environment, name)
	gate, err = authz.Direct(gate, err)
	return c, gate, err
}

func resourceOf(c entities.Cron) authz.Resource {
	return authz.Resource{TeamID: c.TeamID, Environment: c.Environment}
}

func (u CronsUsecase) publish(ctx context.Context, a actor.Actor, gate authz.Gate, action string, c entities.Cron, details map[string]any) {
	u.publisher.Publish(ctx, events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action, TargetKind: events.TargetCron,
		TargetID: c.ID, TargetName: c.Name, TeamID: c.TeamID, Environment: c.Environment, Details: events.WithApproval(a, details),
	})
	authz.PublishBypass(ctx, u.publisher, gate, a, authz.EditCron, resourceOf(c))
}
