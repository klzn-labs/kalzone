package users

import (
	"context"

	identity "github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/team/entities"
)

type UserFinder interface {
	FindByLogin(ctx context.Context, login string) (identity.User, error)
	FindByIDs(ctx context.Context, ids []string) ([]identity.User, error)
}

type Directory struct {
	users UserFinder
}

func NewDirectory(users UserFinder) Directory {
	return Directory{users: users}
}

func (d Directory) FindByLogin(ctx context.Context, login string) (entities.UserRef, error) {
	user, err := d.users.FindByLogin(ctx, login)
	return toRef(user), err
}

func (d Directory) FindByIDs(ctx context.Context, ids []string) ([]entities.UserRef, error) {
	found, err := d.users.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	refs := make([]entities.UserRef, 0, len(found))
	for _, u := range found {
		refs = append(refs, toRef(u))
	}
	return refs, nil
}

func toRef(u identity.User) entities.UserRef {
	return entities.UserRef{ID: u.ID, Login: u.Login, Name: u.Name, AvatarURL: u.AvatarURL}
}
