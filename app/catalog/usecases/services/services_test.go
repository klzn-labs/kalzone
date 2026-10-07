package services_test

import (
	"context"
	"testing"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/app/catalog/usecases/services"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/events"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()
var ana = actor.Actor{UserID: "ana", Login: "ana"}

type store struct{ bySlug map[string]entities.Service }

func (s *store) Create(_ context.Context, svc entities.Service) error {
	s.bySlug[svc.Slug] = svc
	return nil
}

func (s *store) Update(_ context.Context, svc entities.Service) error {
	s.bySlug[svc.Slug] = svc
	return nil
}

func (s *store) FindBySlug(_ context.Context, slug string) (entities.Service, error) {
	svc, ok := s.bySlug[slug]
	if !ok || svc.DeletedAt != nil {
		return entities.Service{}, apperror.NotFound("service not found")
	}
	return svc, nil
}

func (s *store) FindByID(context.Context, string) (entities.Service, error) {
	return entities.Service{}, nil
}
func (s *store) List(context.Context, entities.ServiceFilter) ([]entities.Service, error) {
	return nil, nil
}

type teamIDs map[string]string

func (t teamIDs) IDBySlug(_ context.Context, slug string) (string, error) {
	id, ok := t[slug]
	if !ok {
		return "", apperror.NotFound("team not found")
	}
	return id, nil
}

type checks struct {
	asked   []authz.Resource
	actions []authz.Action
	deny    bool
}

func (c *checks) Can(_ context.Context, _ actor.Actor, action authz.Action, r authz.Resource) error {
	c.asked, c.actions = append(c.asked, r), append(c.actions, action)
	if c.deny {
		return apperror.Forbidden("no")
	}
	return nil
}

type published struct{ events []events.Event }

func (p *published) Publish(_ context.Context, e events.Event) { p.events = append(p.events, e) }

func withWorkflow(svc entities.Service) entities.Service {
	svc.BuildWorkflow = entities.DefaultBuildWorkflow
	return svc
}

func setup(existing ...entities.Service) (*store, *checks, *published, services.ServicesUsecase) {
	s, c, p := &store{bySlug: map[string]entities.Service{}}, &checks{}, &published{}
	for _, svc := range existing {
		s.bySlug[svc.Slug] = withWorkflow(svc)
	}
	return s, c, p, services.NewServicesUsecase(s, teamIDs{"places": "t1", "reviews": "t2"}, c, p)
}

func Test_Create_ChecksTheOwnerTeamAndPublishes(t *testing.T) {
	s, c, p, usecase := setup()

	svc, err := usecase.Create(ctx, ana, entities.NewService{Slug: "api", Name: " API ", OwnerTeam: "places"})

	require.NoError(t, err)
	require.Equal(t, "t1", s.bySlug["api"].OwnerTeamID)
	require.Equal(t, "API", svc.Name)
	require.Equal(t, []authz.Resource{{TeamID: "t1"}}, c.asked)
	require.Equal(t, authz.CreateService, c.actions[0])
	require.Equal(t, "service.created", p.events[0].Action)
	require.Equal(t, "t1", p.events[0].TeamID)
}

func Test_Create_RejectsInvalidInputUnknownTeamAndDenial(t *testing.T) {
	_, _, _, usecase := setup()
	_, c, _, denied := setup()
	c.deny = true

	_, invalid := usecase.Create(ctx, ana, entities.NewService{Slug: "API", Name: "x", OwnerTeam: "places"})
	_, unknown := usecase.Create(ctx, ana, entities.NewService{Slug: "api", Name: "x", OwnerTeam: "ghosts"})
	_, forbidden := denied.Create(ctx, ana, entities.NewService{Slug: "api", Name: "x", OwnerTeam: "places"})

	require.ErrorIs(t, invalid, apperror.Invalid(""))
	require.ErrorIs(t, unknown, apperror.NotFound(""))
	require.ErrorIs(t, forbidden, apperror.Forbidden(""))
}

func Test_Transfer_IsCheckedAgainstTheCurrentOwner(t *testing.T) {
	s, c, p, usecase := setup(entities.Service{ID: "s1", Slug: "api", Name: "API", OwnerTeamID: "t1"})

	_, err := usecase.Transfer(ctx, ana, "api", "reviews")

	require.NoError(t, err)
	require.Equal(t, authz.Resource{TeamID: "t1"}, c.asked[0])
	require.Equal(t, authz.TransferService, c.actions[0])
	require.Equal(t, "t2", s.bySlug["api"].OwnerTeamID)
	require.Equal(t, "service.transferred", p.events[0].Action)
}

func Test_Delete_HidesTheService(t *testing.T) {
	_, c, _, usecase := setup(entities.Service{ID: "s1", Slug: "api", Name: "API", OwnerTeamID: "t1"})

	require.NoError(t, usecase.Delete(ctx, ana, "api"))

	_, err := usecase.Get(ctx, "api")
	require.ErrorIs(t, err, apperror.NotFound(""))
	require.Equal(t, authz.DeleteService, c.actions[0])
}

func Test_Update_ValidatesTheResult(t *testing.T) {
	_, _, _, usecase := setup(entities.Service{ID: "s1", Slug: "api", Name: "API", OwnerTeamID: "t1"})
	bad := "not a repo"

	_, err := usecase.Update(ctx, ana, "api", entities.ServiceChanges{GitHubRepo: &bad})

	require.ErrorIs(t, err, apperror.Invalid(""))
}
