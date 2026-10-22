package members_test

import (
	"context"
	"testing"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/app/team/usecases/members"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type store struct{ roles map[string]entities.Role }

func (s *store) PutMembership(_ context.Context, m entities.Membership) error {
	s.roles[m.UserID] = m.Role
	return nil
}

func (s *store) DeleteMembership(_ context.Context, _, userID string) error {
	delete(s.roles, userID)
	return nil
}

func (s *store) Membership(_ context.Context, teamID, userID string) (entities.Membership, bool, error) {
	role, ok := s.roles[userID]
	return entities.Membership{TeamID: teamID, UserID: userID, Role: role}, ok, nil
}

func (s *store) MembershipsOfTeam(_ context.Context, teamID string) ([]entities.Membership, error) {
	result := []entities.Membership{}
	for userID, role := range s.roles {
		result = append(result, entities.Membership{TeamID: teamID, UserID: userID, Role: role})
	}
	return result, nil
}

func (s *store) CountMaintainers(context.Context, string) (int64, error) {
	var count int64
	for _, role := range s.roles {
		if role == entities.RoleMaintainer {
			count++
		}
	}
	return count, nil
}

type teams struct{}

func (teams) FindBySlug(_ context.Context, slug string) (entities.Team, error) {
	if slug != "places" {
		return entities.Team{}, apperror.NotFound("team not found")
	}
	return entities.Team{ID: "t1", Slug: "places"}, nil
}

type users struct{}

func (users) FindByLogin(_ context.Context, login string) (entities.UserRef, error) {
	return entities.UserRef{ID: login, Login: login}, nil
}

func (users) FindByIDs(_ context.Context, ids []string) ([]entities.UserRef, error) {
	result := []entities.UserRef{}
	for _, id := range ids {
		result = append(result, entities.UserRef{ID: id, Login: id})
	}
	return result, nil
}

type allow struct{ err error }

func (a allow) Can(context.Context, actor.Actor, authz.Action, authz.Resource) error { return a.err }

type published struct{ events []events.Event }

func (p *published) Publish(_ context.Context, e events.Event) { p.events = append(p.events, e) }

var maintainer = actor.Actor{UserID: "mia", Login: "mia", Teams: []string{"t1"}}
var admin = actor.Actor{UserID: "root", Login: "root", Admin: true}

func setup(roles map[string]entities.Role, err error) (*store, *published, members.MembersUsecase) {
	s, p := &store{roles: roles}, &published{}
	return s, p, members.NewMembersUsecase(s, teams{}, users{}, allow{err}, p)
}

func Test_Put_AddsAMemberAndPublishes(t *testing.T) {
	s, p, usecase := setup(map[string]entities.Role{"mia": entities.RoleMaintainer}, nil)

	member, err := usecase.Put(ctx, maintainer, "places", "ana", entities.RoleMember)

	require.NoError(t, err)
	require.Equal(t, "ana", member.User.Login)
	require.Equal(t, entities.RoleMember, s.roles["ana"])
	require.Equal(t, "team.member_put", p.events[0].Action)
	require.Equal(t, "t1", p.events[0].TeamID)
}

func Test_Put_RejectsUnknownRoleAndDeniedActor(t *testing.T) {
	_, _, usecase := setup(map[string]entities.Role{}, nil)
	_, _, denied := setup(map[string]entities.Role{}, apperror.Forbidden("no"))

	_, roleErr := usecase.Put(ctx, maintainer, "places", "ana", entities.Role("owner"))
	_, deniedErr := denied.Put(ctx, maintainer, "places", "ana", entities.RoleMember)

	require.ErrorIs(t, roleErr, apperror.Invalid(""))
	require.ErrorIs(t, deniedErr, apperror.Forbidden(""))
}

func Test_TheLastMaintainerCannotBeDemotedOrRemovedByANonAdmin(t *testing.T) {
	_, _, usecase := setup(map[string]entities.Role{"mia": entities.RoleMaintainer, "ana": entities.RoleMember}, nil)
	lastMaintainer := apperror.New(apperror.CodeLastMaintainer, "")

	_, demoteErr := usecase.Put(ctx, maintainer, "places", "mia", entities.RoleMember)
	removeErr := usecase.Remove(ctx, maintainer, "places", "mia")

	require.ErrorIs(t, demoteErr, lastMaintainer)
	require.ErrorIs(t, removeErr, lastMaintainer)
}

func Test_AMaintainerCanStepDownWhenAnotherRemains(t *testing.T) {
	s, _, usecase := setup(map[string]entities.Role{"mia": entities.RoleMaintainer, "ana": entities.RoleMaintainer}, nil)

	_, err := usecase.Put(ctx, maintainer, "places", "mia", entities.RoleMember)

	require.NoError(t, err)
	require.Equal(t, entities.RoleMember, s.roles["mia"])
}

func Test_AnAdminCanRemoveTheLastMaintainer(t *testing.T) {
	s, _, usecase := setup(map[string]entities.Role{"mia": entities.RoleMaintainer}, nil)

	require.NoError(t, usecase.Remove(ctx, admin, "places", "mia"))

	require.Empty(t, s.roles)
}

func Test_Remove_SomeoneWhoIsNotAMemberIsNotFound(t *testing.T) {
	_, _, usecase := setup(map[string]entities.Role{}, nil)

	err := usecase.Remove(ctx, maintainer, "places", "ghost")

	require.ErrorIs(t, err, apperror.NotFound(""))
}

func Test_List_ResolvesUsers(t *testing.T) {
	_, _, usecase := setup(map[string]entities.Role{"mia": entities.RoleMaintainer}, nil)

	list, err := usecase.List(ctx, maintainer, "places")

	require.NoError(t, err)
	require.Equal(t, "mia", list[0].User.Login)
	require.Equal(t, entities.RoleMaintainer, list[0].Role)
}

func Test_List_HidesATeamFromAnOutsider(t *testing.T) {
	_, _, usecase := setup(map[string]entities.Role{"mia": entities.RoleMaintainer}, nil)

	_, err := usecase.List(ctx, actor.Actor{UserID: "zed", Login: "zed"}, "places")

	require.ErrorIs(t, err, apperror.NotFound(""))
}

func Test_AnAdminsTokenCannotRemoveTheLastMaintainer(t *testing.T) {
	_, _, usecase := setup(map[string]entities.Role{"mia": entities.RoleMaintainer}, nil)
	adminToken := actor.Actor{UserID: "root", Login: "root", Admin: true, Token: actor.Token{ID: "tk1"}}

	err := usecase.Remove(ctx, adminToken, "places", "mia")

	require.ErrorIs(t, err, apperror.New(apperror.CodeLastMaintainer, ""))
}
