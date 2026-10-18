package teams_test

import (
	"context"
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/app/team/usecases/teams"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()
var admin = actor.Actor{UserID: "root", Login: "root", Admin: true}

type store struct{ bySlug map[string]entities.Team }

func (s *store) Create(_ context.Context, t entities.Team) error {
	s.bySlug[t.Slug] = t
	return nil
}

func (s *store) Delete(_ context.Context, id string) error {
	for slug, t := range s.bySlug {
		if t.ID == id {
			delete(s.bySlug, slug)
		}
	}
	return nil
}

func (s *store) FindBySlug(_ context.Context, slug string) (entities.Team, error) {
	t, ok := s.bySlug[slug]
	if !ok {
		return entities.Team{}, apperror.NotFound("team not found")
	}
	return t, nil
}

func (s *store) List(context.Context) ([]entities.Team, error)                { return nil, nil }
func (s *store) FindByIDs(context.Context, []string) ([]entities.Team, error) { return nil, nil }

type services int64

func (s services) CountOwnedBy(context.Context, string) (int64, error) { return int64(s), nil }

type allow struct{ err error }

func (a allow) Can(context.Context, actor.Actor, authz.Action, authz.Resource) error { return a.err }

type published struct{ events []events.Event }

func (p *published) Publish(_ context.Context, e events.Event) { p.events = append(p.events, e) }

func Test_Create_ValidatesAndPublishes(t *testing.T) {
	s, p := &store{bySlug: map[string]entities.Team{}}, &published{}
	usecase := teams.NewTeamsUsecase(s, services(0), allow{}, p)

	team, err := usecase.Create(ctx, admin, "places", "  Places  ")
	_, invalid := usecase.Create(ctx, admin, "Places!", "x")

	require.NoError(t, err)
	require.Equal(t, "Places", team.Name)
	require.NotEmpty(t, team.ID)
	require.Equal(t, "team.created", p.events[0].Action)
	require.ErrorIs(t, invalid, apperror.Invalid(""))
}

func Test_Create_RequiresPermission(t *testing.T) {
	usecase := teams.NewTeamsUsecase(&store{bySlug: map[string]entities.Team{}}, services(0), allow{apperror.Forbidden("no")}, &published{})

	_, err := usecase.Create(ctx, actor.Actor{UserID: "u"}, "places", "Places")

	require.ErrorIs(t, err, apperror.Forbidden(""))
}

func Test_Delete_RefusesWhileTheTeamOwnsServices(t *testing.T) {
	s := &store{bySlug: map[string]entities.Team{"places": {ID: "t1", Slug: "places"}}}
	busy := teams.NewTeamsUsecase(s, services(2), allow{}, &published{})
	idle := teams.NewTeamsUsecase(s, services(0), allow{}, &published{})

	require.ErrorIs(t, busy.Delete(ctx, admin, "places"), apperror.Conflict(""))
	require.NoError(t, idle.Delete(ctx, admin, "places"))
	require.Empty(t, s.bySlug)
}

type asked struct{ resources []authz.Resource }

func (a *asked) Can(_ context.Context, _ actor.Actor, _ authz.Action, r authz.Resource) error {
	a.resources = append(a.resources, r)
	return nil
}

func Test_Delete_IsCheckedAgainstTheTeamBeingDeleted(t *testing.T) {
	s := &store{bySlug: map[string]entities.Team{"reviews": {ID: "t2", Slug: "reviews"}}}
	checks := &asked{}

	require.NoError(t, teams.NewTeamsUsecase(s, services(0), checks, &published{}).Delete(ctx, admin, "reviews"))

	require.Equal(t, []authz.Resource{{TeamID: "t2"}}, checks.resources)
}
