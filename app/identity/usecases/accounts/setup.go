package accounts

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/klzn-labs/kalzone/pkg/scope"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

const (
	defaultTeamSlug = "default"
	defaultTeamName = "Default"
	agentTokenName  = "agent"
	agentTokenDays  = 90
)

var (
	errLinkGone        = apperror.NotFound("This link is no longer valid.")
	errHasOwner        = apperror.Conflict("this Kalzone already has an owner")
	errNotResettable   = apperror.Conflict("reset links are only for active password accounts")
	errPasswordsDiffer = apperror.Invalid("the passwords do not match").WithField("confirm")
)

type LinkStore interface {
	Create(ctx context.Context, link entities.AccountLink) error
	FindByHash(ctx context.Context, hash string) (entities.AccountLink, error)
	RevokeUnused(ctx context.Context, kind entities.LinkKind, at time.Time) error
	RevokeUnusedFor(ctx context.Context, kind entities.LinkKind, userID string, at time.Time) error
}

type AccountStore interface {
	CompleteSetup(ctx context.Context, linkID string, owner entities.User, team entities.NewTeam) error
	CountPeople(ctx context.Context) (int, error)
}

type UserStore interface {
	FindByLogin(ctx context.Context, login string) (entities.User, error)
}

type SessionStore interface {
	Create(ctx context.Context, session entities.Session) error
}

type TokenIssuer interface {
	Create(ctx context.Context, a actor.Actor, d entities.TokenDraft) (entities.IssuedToken, error)
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

type Logger interface {
	Printf(format string, v ...any)
}

type SetupInput struct {
	Username string
	Password string
	Confirm  string
	Bypass   bool
}

type SetupResult struct {
	Session    entities.IssuedSession
	AgentToken string
	MCPCommand string
}

type SetupUsecase struct {
	baseURL    string
	sessionTTL time.Duration
	mode       Mode
	links      LinkStore
	accounts   AccountStore
	users      UserStore
	sessions   SessionStore
	tokens     TokenIssuer
	publisher  Publisher
	logger     Logger
}

func NewSetupUsecase(
	cfg config.Config, mode Mode, links LinkStore, accounts AccountStore, users UserStore,
	sessions SessionStore, tokens TokenIssuer, publisher Publisher, logger Logger,
) SetupUsecase {
	return SetupUsecase{
		baseURL: strings.TrimSuffix(cfg.BaseURL, "/"), sessionTTL: cfg.Session.TTL, mode: mode,
		links: links, accounts: accounts, users: users, sessions: sessions,
		tokens: tokens, publisher: publisher, logger: logger,
	}
}

func (u SetupUsecase) EnsureSetupLink(ctx context.Context) error {
	if u.mode.GitHub {
		return nil
	}
	people, err := u.accounts.CountPeople(ctx)
	if err != nil || people > 0 {
		return err
	}
	url, err := u.NewSetupLink(ctx)
	if err != nil {
		return err
	}
	u.logger.Printf("WARN setup: open %s to create the owner", url)
	return nil
}

func (u SetupUsecase) NewSetupLink(ctx context.Context) (string, error) {
	if err := u.mode.RequirePasswords(); err != nil {
		return "", err
	}
	if err := u.requireNoPeople(ctx); err != nil {
		return "", err
	}
	if err := u.links.RevokeUnused(ctx, entities.LinkSetup, clock.Now().UTC()); err != nil {
		return "", err
	}
	_, url, err := u.linker().issue(ctx, entities.AccountLink{Kind: entities.LinkSetup})
	return url, err
}

func (u SetupUsecase) linker() linkIssuer {
	return linkIssuer{baseURL: u.baseURL, links: u.links}
}

func (u SetupUsecase) requireNoPeople(ctx context.Context) error {
	people, err := u.accounts.CountPeople(ctx)
	if err != nil {
		return err
	}
	if people > 0 {
		return errHasOwner
	}
	return nil
}

func (u SetupUsecase) NewResetLinkFor(ctx context.Context, login string) (string, error) {
	if err := u.mode.RequirePasswords(); err != nil {
		return "", err
	}
	user, err := u.users.FindByLogin(ctx, strings.ToLower(strings.TrimSpace(login)))
	if err != nil {
		return "", err
	}
	url, err := u.linker().reset(ctx, user, "")
	if err != nil {
		return "", err
	}
	u.publisher.Publish(ctx, userEvent(actor.Actor{}, "user.password_reset_issued", user))
	return url, nil
}

func (u SetupUsecase) Complete(ctx context.Context, token string, in SetupInput) (SetupResult, error) {
	link, err := u.openSetupLink(ctx, token)
	if err != nil {
		return SetupResult{}, err
	}
	owner, team, err := newOwner(in)
	if err != nil {
		return SetupResult{}, err
	}
	if err := u.accounts.CompleteSetup(ctx, link.ID, owner, team); err != nil {
		return SetupResult{}, err
	}
	u.publisher.Publish(ctx, setupCompleted(owner, team))
	result, err := u.signInOwner(ctx, owner, team, in.Bypass)
	if err != nil {
		return SetupResult{}, SetupUnfinishedError{Err: err}
	}
	return result, nil
}

func (u SetupUsecase) openSetupLink(ctx context.Context, token string) (entities.AccountLink, error) {
	if err := u.mode.RequirePasswords(); err != nil {
		return entities.AccountLink{}, err
	}
	link, err := u.linker().live(ctx, token, entities.LinkSetup)
	if err != nil {
		return entities.AccountLink{}, err
	}
	return link, u.requireNoPeople(ctx)
}

func newOwner(in SetupInput) (entities.User, entities.NewTeam, error) {
	owner, err := newPerson(in)
	if err != nil {
		return entities.User{}, entities.NewTeam{}, err
	}
	owner.Admin = true
	team := entities.NewTeam{ID: uuid.NewUUID(), Slug: defaultTeamSlug, Name: defaultTeamName, CreatedAt: owner.CreatedAt}
	return owner, team, nil
}

func checkedCredentials(in SetupInput) (string, string, error) {
	login, err := entities.NormalizeLogin(in.Username)
	if err != nil {
		return "", "", err
	}
	hash, err := checkedPassword(in.Password, in.Confirm)
	return login, hash, err
}

func (u SetupUsecase) signInOwner(ctx context.Context, owner entities.User, team entities.NewTeam, bypass bool) (SetupResult, error) {
	issued, err := startSession(ctx, u.sessions, owner.ID, u.sessionTTL)
	if err != nil {
		return SetupResult{}, err
	}
	ownerActor := actor.Actor{UserID: owner.ID, Login: owner.Login, Admin: true, Teams: []string{team.ID}}
	agent, err := u.tokens.Create(ctx, ownerActor, agentDraft(bypass))
	if err != nil {
		return SetupResult{}, err
	}
	return SetupResult{Session: issued, AgentToken: agent.Secret, MCPCommand: u.mcpCommand(agent.Secret)}, nil
}

func agentDraft(bypass bool) entities.TokenDraft {
	scopes := make([]string, 0, len(scope.All))
	for _, s := range scope.All {
		scopes = append(scopes, string(s))
	}
	return entities.TokenDraft{
		Name: agentTokenName, Scopes: scopes, Teams: []string{defaultTeamSlug},
		ExpiresInDays: agentTokenDays, Bypass: bypass,
	}
}

func (u SetupUsecase) mcpCommand(secret string) string {
	return fmt.Sprintf(`claude mcp add --transport http kalzone %s/mcp --header "Authorization: Bearer %s"`, u.baseURL, secret)
}

func setupCompleted(owner entities.User, team entities.NewTeam) events.Event {
	return events.Event{
		ActorID: owner.ID, ActorLogin: owner.Login, Action: "setup.completed",
		TargetKind: events.TargetUser, TargetID: owner.ID, TargetName: owner.Login, TeamID: team.ID,
	}
}
