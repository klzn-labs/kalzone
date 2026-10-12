package services

import (
	"context"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/events"
)

type ServiceStore interface {
	Create(ctx context.Context, s entities.Service) error
	Update(ctx context.Context, s entities.Service) error
	FindBySlug(ctx context.Context, slug string) (entities.Service, error)
	FindByID(ctx context.Context, id string) (entities.Service, error)
	List(ctx context.Context, filter entities.ServiceFilter) ([]entities.Service, error)
}

type TeamLookup interface {
	IDBySlug(ctx context.Context, slug string) (string, error)
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

type ServicesUsecase struct {
	store     ServiceStore
	teams     TeamLookup
	authz     Authorizer
	publisher Publisher
}

func NewServicesUsecase(store ServiceStore, teams TeamLookup, authz Authorizer, publisher Publisher) ServicesUsecase {
	return ServicesUsecase{store: store, teams: teams, authz: authz, publisher: publisher}
}

func (u ServicesUsecase) List(ctx context.Context, filter entities.ServiceFilter) ([]entities.Service, error) {
	return u.store.List(ctx, filter)
}

func (u ServicesUsecase) Get(ctx context.Context, slug string) (entities.Service, error) {
	return u.store.FindBySlug(ctx, slug)
}

func (u ServicesUsecase) GetByID(ctx context.Context, id string) (entities.Service, error) {
	return u.store.FindByID(ctx, id)
}

func serviceEvent(a actor.Actor, action string, s entities.Service, details map[string]any) events.Event {
	return events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action,
		TargetKind: events.TargetService, TargetID: s.ID, TargetName: s.Slug, TeamID: s.OwnerTeamID,
		Details: details,
	}
}

func (u ServicesUsecase) ListOwnedBy(ctx context.Context, teamSlug string) ([]entities.Service, error) {
	teamID, err := u.teams.IDBySlug(ctx, teamSlug)
	if err != nil {
		return nil, err
	}
	return u.store.List(ctx, entities.ServiceFilter{OwnerTeamIDs: []string{teamID}})
}

func (u ServicesUsecase) ForRepository(ctx context.Context, githubRepo string) ([]entities.Service, error) {
	return u.store.List(ctx, entities.ServiceFilter{GitHubRepo: githubRepo})
}
