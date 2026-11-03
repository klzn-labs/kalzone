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
	"github.com/klzn-labs/kalzone/pkg/events"
)

var (
	errSignedIn       = apperror.Conflict("Sign out to accept this invite")
	errInviteRole     = apperror.Invalid("role must be member or maintainer").WithField("role")
	errInviteNotFound = apperror.NotFound("invite not found")
)

type InviteLinks interface {
	LinkStore
	ListInvites(ctx context.Context, teamID string, now time.Time) ([]entities.AccountLink, error)
	Revoke(ctx context.Context, id string, at time.Time) error
}

type InviteAccounts interface {
	AcceptInvite(ctx context.Context, link entities.AccountLink, user entities.User) error
}

type TeamLookup interface {
	IDBySlug(ctx context.Context, slug string) (string, error)
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type Invite struct {
	Link entities.AccountLink
	URL  string
}

type InvitesUsecase struct {
	sessionTTL time.Duration
	mode       Mode
	linker     linkIssuer
	links      InviteLinks
	accounts   InviteAccounts
	teams      TeamLookup
	authz      Authorizer
	sessions   SessionStore
	publisher  Publisher
}

func NewInvitesUsecase(
	cfg config.Config, mode Mode, links InviteLinks, accounts InviteAccounts, teams TeamLookup,
	authz Authorizer, sessions SessionStore, publisher Publisher,
) InvitesUsecase {
	return InvitesUsecase{
		sessionTTL: cfg.Session.TTL, mode: mode, linker: linkIssuer{baseURL: strings.TrimSuffix(cfg.BaseURL, "/"), links: links},
		links: links, accounts: accounts, teams: teams, authz: authz, sessions: sessions, publisher: publisher,
	}
}

func (u InvitesUsecase) Create(ctx context.Context, a actor.Actor, teamSlug, role string) (Invite, error) {
	teamID, err := u.authorize(ctx, a, teamSlug)
	if err != nil {
		return Invite{}, err
	}
	if role != string(authz.RoleMember) && role != string(authz.RoleMaintainer) {
		return Invite{}, errInviteRole
	}
	draft := entities.AccountLink{Kind: entities.LinkInvite, TeamID: teamID, Role: role, CreatedBy: a.UserID}
	link, url, err := u.linker.issue(ctx, draft)
	if err != nil {
		return Invite{}, err
	}
	u.publisher.Publish(ctx, inviteEvent(a, "invite.created", teamSlug, link))
	return Invite{Link: link, URL: url}, nil
}

func (u InvitesUsecase) List(ctx context.Context, a actor.Actor, teamSlug string) ([]entities.AccountLink, error) {
	teamID, err := u.authorize(ctx, a, teamSlug)
	if err != nil {
		return nil, err
	}
	return u.links.ListInvites(ctx, teamID, clock.Now().UTC())
}

func (u InvitesUsecase) Revoke(ctx context.Context, a actor.Actor, teamSlug, id string) error {
	teamID, err := u.authorize(ctx, a, teamSlug)
	if err != nil {
		return err
	}
	link, err := u.pending(ctx, teamID, id)
	if err != nil {
		return err
	}
	if err := u.links.Revoke(ctx, link.ID, clock.Now().UTC()); err != nil {
		return err
	}
	u.publisher.Publish(ctx, inviteEvent(a, "invite.revoked", teamSlug, link))
	return nil
}

func (u InvitesUsecase) Accept(ctx context.Context, token, username, password, confirm string, signedIn bool) (entities.IssuedSession, error) {
	if err := u.mode.RequirePasswords(); err != nil {
		return entities.IssuedSession{}, err
	}
	if signedIn {
		return entities.IssuedSession{}, errSignedIn
	}
	link, err := u.linker.live(ctx, token, entities.LinkInvite)
	if err != nil {
		return entities.IssuedSession{}, err
	}
	user, err := newPerson(SetupInput{Username: username, Password: password, Confirm: confirm})
	if err != nil {
		return entities.IssuedSession{}, err
	}
	if err := u.accounts.AcceptInvite(ctx, link, user); err != nil {
		return entities.IssuedSession{}, err
	}
	u.publisher.Publish(ctx, inviteAccepted(user, link))
	return startSession(ctx, u.sessions, user.ID, u.sessionTTL)
}

func (u InvitesUsecase) authorize(ctx context.Context, a actor.Actor, teamSlug string) (string, error) {
	if err := u.mode.RequirePasswords(); err != nil {
		return "", err
	}
	teamID, err := u.teams.IDBySlug(ctx, teamSlug)
	if err != nil {
		return "", err
	}
	return teamID, u.authz.Can(ctx, a, authz.ManageInvites, authz.Resource{TeamID: teamID})
}

func (u InvitesUsecase) pending(ctx context.Context, teamID, id string) (entities.AccountLink, error) {
	invites, err := u.links.ListInvites(ctx, teamID, clock.Now().UTC())
	if err != nil {
		return entities.AccountLink{}, err
	}
	for _, link := range invites {
		if link.ID == id {
			return link, nil
		}
	}
	return entities.AccountLink{}, errInviteNotFound
}

func inviteEvent(a actor.Actor, action, teamSlug string, link entities.AccountLink) events.Event {
	return events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action,
		TargetKind: events.TargetTeam, TargetID: link.TeamID, TargetName: teamSlug, TeamID: link.TeamID,
		Details: map[string]any{"invite_id": link.ID, "role": link.Role},
	}
}

func inviteAccepted(user entities.User, link entities.AccountLink) events.Event {
	event := userEvent(actorOf(user), "invite.accepted", user)
	event.TeamID = link.TeamID
	event.Details = map[string]any{"invite_id": link.ID, "role": link.Role}
	return event
}
