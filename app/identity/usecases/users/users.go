package users

import (
	"context"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/events"
)

var errRemovedPerson = apperror.Conflict("this person was removed")

type UserStore interface {
	List(ctx context.Context) ([]entities.User, error)
	FindByLogin(ctx context.Context, login string) (entities.User, error)
	FindByIDs(ctx context.Context, ids []string) ([]entities.User, error)
	Save(ctx context.Context, user entities.User) error
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type CoMembers interface {
	UserIDsIn(ctx context.Context, teamIDs []string) ([]string, error)
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

type UsersUsecase struct {
	users     UserStore
	members   CoMembers
	authz     Authorizer
	publisher Publisher
}

func NewUsersUsecase(users UserStore, members CoMembers, authz Authorizer, publisher Publisher) UsersUsecase {
	return UsersUsecase{users: users, members: members, authz: authz, publisher: publisher}
}

func (u UsersUsecase) List(ctx context.Context) ([]entities.User, error) {
	return u.users.List(ctx)
}

func (u UsersUsecase) FindByLogin(ctx context.Context, login string) (entities.User, error) {
	return u.users.FindByLogin(ctx, login)
}

func (u UsersUsecase) FindByIDs(ctx context.Context, ids []string) ([]entities.User, error) {
	if len(ids) == 0 {
		return []entities.User{}, nil
	}
	return u.users.FindByIDs(ctx, ids)
}

func (u UsersUsecase) SetAdmin(ctx context.Context, a actor.Actor, login string, admin bool) (entities.User, error) {
	if err := u.authz.Can(ctx, a, authz.ManageAdmins, authz.Resource{}); err != nil {
		return entities.User{}, err
	}
	user, err := u.users.FindByLogin(ctx, login)
	if err != nil {
		return entities.User{}, err
	}
	if user.Disabled() {
		return entities.User{}, errRemovedPerson
	}
	user.Admin = admin
	if err := u.users.Save(ctx, user); err != nil {
		return entities.User{}, err
	}
	u.publisher.Publish(ctx, adminChanged(a, user))
	return user, nil
}

func adminChanged(a actor.Actor, user entities.User) events.Event {
	action := "user.admin_revoked"
	if user.Admin {
		action = "user.admin_granted"
	}
	return events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action,
		TargetKind: events.TargetUser, TargetID: user.ID, TargetName: user.Login,
	}
}
