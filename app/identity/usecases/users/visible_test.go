package users_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/usecases/users"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

type store []entities.User

func (s store) List(context.Context) ([]entities.User, error) { return s, nil }

func (s store) FindByLogin(_ context.Context, login string) (entities.User, error) {
	for _, u := range s {
		if u.Login == login {
			return u, nil
		}
	}
	return entities.User{}, apperror.NotFound("user not found")
}

func (s store) FindByIDs(_ context.Context, ids []string) ([]entities.User, error) {
	return slices.DeleteFunc(slices.Clone(s), func(u entities.User) bool { return !slices.Contains(ids, u.ID) }), nil
}

func (s store) Save(context.Context, entities.User) error { return nil }

type coMembers map[string][]string

func (c coMembers) UserIDsIn(_ context.Context, teamIDs []string) ([]string, error) {
	ids := []string{}
	for _, id := range teamIDs {
		ids = append(ids, c[id]...)
	}
	return ids, nil
}

type allow struct{}

func (allow) Can(context.Context, actor.Actor, authz.Action, authz.Resource) error { return nil }

type discard struct{}

func (discard) Publish(context.Context, events.Event) {}

func usecase() users.UsersUsecase {
	people := store{{ID: "u1", Login: "ana"}, {ID: "u2", Login: "bob"}, {ID: "u3", Login: "zed"}}
	return users.NewUsersUsecase(people, coMembers{"t1": {"u1", "u2"}}, allow{}, discard{})
}

func Test_ListFor_ShowsPeopleWhoShareATeam(t *testing.T) {
	ana := actor.Actor{UserID: "u1", Teams: []string{"t1"}}

	shared, err := usecase().ListFor(ctx, ana)
	all, _ := usecase().ListFor(ctx, actor.Actor{UserID: "u9", Admin: true})
	alone, _ := usecase().ListFor(ctx, actor.Actor{UserID: "u3"})

	require.NoError(t, err)
	require.Equal(t, []string{"ana", "bob"}, logins(shared))
	require.Len(t, all, 3)
	require.Equal(t, []string{"zed"}, logins(alone))
}

func Test_GetFor_HidesSomeoneWhoSharesNoTeam(t *testing.T) {
	ana := actor.Actor{UserID: "u1", Teams: []string{"t1"}}

	bob, err := usecase().GetFor(ctx, ana, "bob")
	_, hidden := usecase().GetFor(ctx, ana, "zed")

	require.NoError(t, err)
	require.Equal(t, "u2", bob.ID)
	require.ErrorIs(t, hidden, apperror.NotFound(""))
}

func logins(list []entities.User) []string {
	result := []string{}
	for _, u := range list {
		result = append(result, u.Login)
	}
	return result
}

func Test_SetAdmin_RefusesARemovedPerson(t *testing.T) {
	gone := now()
	people := store{{ID: "u1", Login: "gone", DisabledAt: &gone}}
	p := &recorded{}

	_, err := users.NewUsersUsecase(people, coMembers{}, allow{}, p).SetAdmin(ctx, actor.Actor{UserID: "root", Admin: true}, "gone", true)

	require.ErrorIs(t, err, apperror.Conflict(""))
	require.EqualError(t, err, "this person was removed")
	require.Empty(t, p.events)
}

type recorded struct{ events []events.Event }

func (r *recorded) Publish(_ context.Context, e events.Event) { r.events = append(r.events, e) }

func now() time.Time { return time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC) }
