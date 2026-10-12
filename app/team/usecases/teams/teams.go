package teams

import (
	"context"
	"strings"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/team/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/events"
	"github.com/adrianozp/platform/pkg/slug"
	"github.com/adrianozp/platform/pkg/uuid"
)

type TeamStore interface {
	Create(ctx context.Context, team entities.Team) error
	Delete(ctx context.Context, id string) error
	FindBySlug(ctx context.Context, slug string) (entities.Team, error)
	List(ctx context.Context) ([]entities.Team, error)
	FindByIDs(ctx context.Context, ids []string) ([]entities.Team, error)
}

type ServiceCounter interface {
	CountOwnedBy(ctx context.Context, teamID string) (int64, error)
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

var errStillOwnsServices = apperror.Conflict("move or delete the team's services first")

type TeamsUsecase struct {
	store     TeamStore
	services  ServiceCounter
	authz     Authorizer
	publisher Publisher
}

func NewTeamsUsecase(store TeamStore, services ServiceCounter, authz Authorizer, publisher Publisher) TeamsUsecase {
	return TeamsUsecase{store: store, services: services, authz: authz, publisher: publisher}
}

func (u TeamsUsecase) Create(ctx context.Context, a actor.Actor, teamSlug, name string) (entities.Team, error) {
	if err := u.authz.Can(ctx, a, authz.ManageTeams, authz.Resource{}); err != nil {
		return entities.Team{}, err
	}
	if err := validate(teamSlug, name); err != nil {
		return entities.Team{}, err
	}
	team := entities.Team{ID: uuid.NewUUID(), Slug: teamSlug, Name: strings.TrimSpace(name), CreatedAt: clock.Now().UTC()}
	if err := u.store.Create(ctx, team); err != nil {
		return entities.Team{}, err
	}
	u.publisher.Publish(ctx, teamEvent(a, "team.created", team))
	return team, nil
}

func (u TeamsUsecase) Delete(ctx context.Context, a actor.Actor, teamSlug string) error {
	team, err := u.store.FindBySlug(ctx, teamSlug)
	if err != nil {
		return err
	}
	if err := u.authz.Can(ctx, a, authz.ManageTeams, authz.Resource{TeamID: team.ID}); err != nil {
		return err
	}
	if err := u.ensureNoServices(ctx, team.ID); err != nil {
		return err
	}
	if err := u.store.Delete(ctx, team.ID); err != nil {
		return err
	}
	u.publisher.Publish(ctx, teamEvent(a, "team.deleted", team))
	return nil
}

func (u TeamsUsecase) ensureNoServices(ctx context.Context, teamID string) error {
	count, err := u.services.CountOwnedBy(ctx, teamID)
	if err != nil {
		return err
	}
	if count > 0 {
		return errStillOwnsServices
	}
	return nil
}

func (u TeamsUsecase) List(ctx context.Context) ([]entities.Team, error) {
	return u.store.List(ctx)
}

func (u TeamsUsecase) Get(ctx context.Context, teamSlug string) (entities.Team, error) {
	return u.store.FindBySlug(ctx, teamSlug)
}

func (u TeamsUsecase) FindByIDs(ctx context.Context, ids []string) ([]entities.Team, error) {
	if len(ids) == 0 {
		return []entities.Team{}, nil
	}
	return u.store.FindByIDs(ctx, ids)
}

func validate(teamSlug, name string) error {
	if err := slug.Validate("team", teamSlug); err != nil {
		return err
	}
	return slug.ValidateName("team", name)
}

func teamEvent(a actor.Actor, action string, team entities.Team) events.Event {
	return events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action,
		TargetKind: events.TargetTeam, TargetID: team.ID, TargetName: team.Slug, TeamID: team.ID,
	}
}
