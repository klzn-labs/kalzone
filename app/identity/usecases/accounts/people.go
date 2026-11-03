package accounts

import (
	"context"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
)

var (
	errLastAdmin      = apperror.Conflict("keep at least one admin")
	errAlreadyRemoved = apperror.Conflict("this person was already removed")
	errWrongPassword  = apperror.Invalid("the current password is wrong").WithField("current")
	errNoPassword     = apperror.Conflict("only password accounts have a password")
	errBrowserOnly    = apperror.Forbidden("sign in to the website to change your password")
)

type PeopleUsers interface {
	FindByLogin(ctx context.Context, login string) (entities.User, error)
	FindByID(ctx context.Context, id string) (entities.User, error)
	List(ctx context.Context) ([]entities.User, error)
}

type PeopleAccounts interface {
	ResetPassword(ctx context.Context, link entities.AccountLink, hash string) error
	Disable(ctx context.Context, userID string, at time.Time) error
	ChangePassword(ctx context.Context, userID, hash, keepSessionHash string) error
}

type FailureClearer interface {
	ClearFailures(login, ip string)
}

type PeopleUsecase struct {
	mode      Mode
	linker    linkIssuer
	accounts  PeopleAccounts
	users     PeopleUsers
	authz     Authorizer
	failures  FailureClearer
	publisher Publisher
}

func NewPeopleUsecase(
	cfg config.Config, mode Mode, links LinkStore, accounts PeopleAccounts, users PeopleUsers,
	authz Authorizer, failures FailureClearer, publisher Publisher,
) PeopleUsecase {
	return PeopleUsecase{
		mode: mode, linker: linkIssuer{baseURL: strings.TrimSuffix(cfg.BaseURL, "/"), links: links},
		accounts: accounts, users: users, authz: authz, failures: failures, publisher: publisher,
	}
}

func (u PeopleUsecase) Remove(ctx context.Context, a actor.Actor, login string) error {
	user, err := u.managed(ctx, a, login)
	if err != nil {
		return err
	}
	if user.Disabled() {
		return errAlreadyRemoved
	}
	if err := u.keepAnAdmin(ctx, user); err != nil {
		return err
	}
	if err := u.accounts.Disable(ctx, user.ID, clock.Now().UTC()); err != nil {
		return err
	}
	u.publisher.Publish(ctx, userEvent(a, "user.removed", user))
	return nil
}

func (u PeopleUsecase) IssueReset(ctx context.Context, a actor.Actor, login string) (string, error) {
	if err := u.mode.RequirePasswords(); err != nil {
		return "", err
	}
	user, err := u.managed(ctx, a, login)
	if err != nil {
		return "", err
	}
	url, err := u.linker.reset(ctx, user, a.UserID)
	if err != nil {
		return "", err
	}
	u.publisher.Publish(ctx, userEvent(a, "user.password_reset_issued", user))
	return url, nil
}

func (u PeopleUsecase) UseReset(ctx context.Context, token, password, confirm, ip string) error {
	link, user, err := u.resetTarget(ctx, token)
	if err != nil {
		return err
	}
	hash, err := checkedPassword(password, confirm)
	if err != nil {
		return err
	}
	if err := u.accounts.ResetPassword(ctx, link, hash); err != nil {
		return err
	}
	u.failures.ClearFailures(user.Login, ip)
	u.publisher.Publish(ctx, userEvent(actorOf(user), "user.password_reset", user))
	return nil
}

func (u PeopleUsecase) ChangePassword(ctx context.Context, a actor.Actor, current, next, confirm, keepSessionHash string) error {
	user, err := u.passwordOwner(ctx, a)
	if err != nil {
		return err
	}
	if !entities.VerifyPassword(user.PasswordHash, current) {
		return errWrongPassword
	}
	hash, err := checkedPassword(next, confirm)
	if err != nil {
		return err
	}
	if err := u.accounts.ChangePassword(ctx, user.ID, hash, keepSessionHash); err != nil {
		return err
	}
	u.publisher.Publish(ctx, userEvent(a, "user.password_changed", user))
	return nil
}

func (u PeopleUsecase) managed(ctx context.Context, a actor.Actor, login string) (entities.User, error) {
	if err := u.authz.Can(ctx, a, authz.ManagePeople, authz.Resource{}); err != nil {
		return entities.User{}, err
	}
	return u.users.FindByLogin(ctx, strings.ToLower(strings.TrimSpace(login)))
}

func (u PeopleUsecase) keepAnAdmin(ctx context.Context, user entities.User) error {
	if !user.Admin {
		return nil
	}
	everyone, err := u.users.List(ctx)
	if err != nil {
		return err
	}
	for _, other := range everyone {
		if other.Admin && !other.Disabled() && other.ID != user.ID {
			return nil
		}
	}
	return errLastAdmin
}

func (u PeopleUsecase) resetTarget(ctx context.Context, token string) (entities.AccountLink, entities.User, error) {
	if err := u.mode.RequirePasswords(); err != nil {
		return entities.AccountLink{}, entities.User{}, err
	}
	link, err := u.linker.live(ctx, token, entities.LinkReset)
	if err != nil || link.UserID == "" {
		return entities.AccountLink{}, entities.User{}, firstErr(err, errLinkGone)
	}
	user, err := u.users.FindByID(ctx, link.UserID)
	if err != nil || user.Disabled() || !user.Local() {
		return entities.AccountLink{}, entities.User{}, firstErr(err, errLinkGone)
	}
	return link, user, nil
}

func (u PeopleUsecase) passwordOwner(ctx context.Context, a actor.Actor) (entities.User, error) {
	if err := u.mode.RequirePasswords(); err != nil {
		return entities.User{}, err
	}
	if !a.SignedIn() || a.ViaToken() {
		return entities.User{}, errBrowserOnly
	}
	user, err := u.users.FindByID(ctx, a.UserID)
	if err != nil {
		return entities.User{}, err
	}
	if user.Disabled() || !user.Local() {
		return entities.User{}, errNoPassword
	}
	return user, nil
}

func firstErr(err, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}
