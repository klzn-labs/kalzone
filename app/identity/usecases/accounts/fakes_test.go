package accounts_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/klzn-labs/kalzone/app/authz"
	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/usecases/accounts"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/events"
)

var (
	ctx      = context.Background()
	now      = time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	baseURL  = "https://kalzone.test"
	errGone  = apperror.NotFound("This link is no longer valid.")
	password = "correct horse battery"
)

type links struct {
	createErr error
	byHash    map[string]entities.AccountLink
	order     []string
	revoked   []entities.LinkKind
}

func (l *links) Create(_ context.Context, link entities.AccountLink) error {
	if l.createErr != nil {
		return l.createErr
	}
	l.byHash[link.TokenHash] = link
	l.order = append(l.order, link.TokenHash)
	return nil
}

func (l *links) FindByHash(_ context.Context, hash string) (entities.AccountLink, error) {
	link, ok := l.byHash[hash]
	if !ok {
		return entities.AccountLink{}, errGone
	}
	return link, nil
}

func (l *links) RevokeUnused(_ context.Context, kind entities.LinkKind, at time.Time) error {
	l.revoked = append(l.revoked, kind)
	for hash, link := range l.byHash {
		if link.Kind == kind && link.UsedAt == nil {
			link.RevokedAt = &at
			l.byHash[hash] = link
		}
	}
	return nil
}

func (l *links) all() []entities.AccountLink {
	result := []entities.AccountLink{}
	for _, hash := range l.order {
		result = append(result, l.byHash[hash])
	}
	return result
}

type store struct {
	links  *links
	people int
	owner  entities.User
	team   entities.NewTeam
}

func (s *store) CountPeople(context.Context) (int, error) {
	return s.people, nil
}

func (s *store) CompleteSetup(_ context.Context, linkID string, owner entities.User, team entities.NewTeam) error {
	for hash, link := range s.links.byHash {
		if link.ID == linkID && link.Live(now) {
			link.UsedAt = &now
			s.links.byHash[hash] = link
			s.owner, s.team, s.people = owner, team, s.people+1
			return nil
		}
	}
	return errGone
}

type users map[string]entities.User

func (u users) FindByLogin(_ context.Context, login string) (entities.User, error) {
	user, ok := u[login]
	if !ok {
		return entities.User{}, apperror.NotFound("user not found")
	}
	return user, nil
}

type sessions struct {
	created []entities.Session
}

func (s *sessions) Create(_ context.Context, session entities.Session) error {
	s.created = append(s.created, session)
	return nil
}

type tokens struct {
	actor actor.Actor
	draft entities.TokenDraft
	err   error
}

func (t *tokens) Create(_ context.Context, a actor.Actor, d entities.TokenDraft) (entities.IssuedToken, error) {
	t.actor, t.draft = a, d
	if t.err != nil {
		return entities.IssuedToken{}, t.err
	}
	return entities.IssuedToken{Token: entities.AccessToken{ID: "tok-1", Name: d.Name}, Secret: "kz_agent_secret"}, nil
}

type publisher struct{ events []events.Event }

func (p *publisher) Publish(_ context.Context, event events.Event) {
	p.events = append(p.events, event)
}

type logger struct {
	mu    sync.Mutex
	lines []string
}

func (l *logger) Printf(format string, v ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, v...))
}

type fixture struct {
	links     *links
	store     *store
	users     users
	sessions  *sessions
	tokens    *tokens
	publisher *publisher
	logger    *logger
	usecase   accounts.SetupUsecase
}

func passwordConfig() config.Config {
	return config.Config{BaseURL: baseURL}
}

func githubConfig() config.Config {
	cfg := passwordConfig()
	cfg.GitHub.ClientID, cfg.GitHub.ClientSecret = "id", "secret"
	return cfg
}

func newFixture(cfg config.Config) *fixture {
	clock.WithTime(now)
	l := &links{byHash: map[string]entities.AccountLink{}}
	f := &fixture{
		links: l, store: &store{links: l}, users: users{}, sessions: &sessions{},
		tokens: &tokens{}, publisher: &publisher{}, logger: &logger{},
	}
	f.usecase = accounts.NewSetupUsecase(cfg, accounts.NewMode(cfg), f.links, f.store, f.users, f.sessions, f.tokens, f.publisher, f.logger)
	return f
}

func (l *links) RevokeUnusedFor(_ context.Context, kind entities.LinkKind, userID string, at time.Time) error {
	for hash, link := range l.byHash {
		if link.Kind == kind && link.UserID == userID && link.UsedAt == nil {
			link.RevokedAt = &at
			l.byHash[hash] = link
		}
	}
	return nil
}

func (l *links) ListInvites(_ context.Context, teamID string, at time.Time) ([]entities.AccountLink, error) {
	result := []entities.AccountLink{}
	for _, link := range l.all() {
		if link.Kind == entities.LinkInvite && link.TeamID == teamID && link.Live(at) {
			result = append(result, link)
		}
	}
	return result, nil
}

func (l *links) Revoke(_ context.Context, id string, at time.Time) error {
	for hash, link := range l.byHash {
		if link.ID == id && link.Live(at) {
			link.RevokedAt = &at
			l.byHash[hash] = link
		}
	}
	return nil
}

func (l *links) claim(id string) error {
	for hash, link := range l.byHash {
		if link.ID == id && link.Live(now) {
			link.UsedAt = &now
			l.byHash[hash] = link
			return nil
		}
	}
	return errGone
}

type accepted struct {
	link entities.AccountLink
	user entities.User
}

type peopleStore struct {
	links    *links
	users    users
	accepted []accepted
	resets   map[string]string
	disabled []string
	changed  []revokedSessions
}

func (s *peopleStore) AcceptInvite(_ context.Context, link entities.AccountLink, user entities.User) error {
	if _, taken := s.users[user.Login]; taken {
		return apperror.Conflict("this username is taken")
	}
	if err := s.links.claim(link.ID); err != nil {
		return err
	}
	s.users[user.Login] = user
	s.accepted = append(s.accepted, accepted{link: link, user: user})
	return nil
}

func (s *peopleStore) ResetPassword(_ context.Context, link entities.AccountLink, hash string) error {
	if err := s.links.claim(link.ID); err != nil {
		return err
	}
	s.resets[link.UserID] = hash
	return nil
}

func (s *peopleStore) Disable(_ context.Context, userID string, at time.Time) error {
	s.disabled = append(s.disabled, userID)
	for login, user := range s.users {
		if user.ID == userID {
			user.DisabledAt, user.Admin = &at, false
			s.users[login] = user
		}
	}
	return nil
}

func (u users) FindByID(_ context.Context, id string) (entities.User, error) {
	for _, user := range u {
		if user.ID == id {
			return user, nil
		}
	}
	return entities.User{}, apperror.NotFound("user not found")
}

func (u users) List(context.Context) ([]entities.User, error) {
	result := []entities.User{}
	for _, user := range u {
		result = append(result, user)
	}
	return result, nil
}

func (s *peopleStore) ChangePassword(_ context.Context, userID, hash, keepSessionHash string) error {
	s.changed = append(s.changed, revokedSessions{userID: userID, keepHash: keepSessionHash})
	for login, user := range s.users {
		if user.ID == userID {
			user.PasswordHash = hash
			s.users[login] = user
		}
	}
	return nil
}

type revokedSessions struct {
	userID   string
	keepHash string
}

type cleared struct{ calls []string }

func (c *cleared) ClearFailures(login, ip string) {
	c.calls = append(c.calls, login+"|"+ip)
}

type teams map[string]string

func (t teams) IDBySlug(_ context.Context, slug string) (string, error) {
	id, ok := t[slug]
	if !ok {
		return "", apperror.NotFound("team not found")
	}
	return id, nil
}

type roles map[string]string

func (r roles) RoleOf(_ context.Context, teamID, userID string) (string, error) {
	return r[teamID+"/"+userID], nil
}

func (roles) IsDemo(context.Context, string) (bool, error) {
	return false, nil
}

func (roles) IsProtected(context.Context, string) (bool, error) {
	return false, nil
}

var (
	admin      = actor.Actor{UserID: "u-admin", Login: "admin", Admin: true}
	maintainer = actor.Actor{UserID: "u-maint", Login: "maint"}
	member     = actor.Actor{UserID: "u-member", Login: "member"}
)

type peopleFixture struct {
	links     *links
	store     *peopleStore
	users     users
	sessions  *sessions
	cleared   *cleared
	publisher *publisher
	invites   accounts.InvitesUsecase
	people    accounts.PeopleUsecase
}

func newPeopleFixture(cfg config.Config) *peopleFixture {
	clock.WithTime(now)
	cfg.Session.TTL = time.Hour
	l, u := &links{byHash: map[string]entities.AccountLink{}}, users{}
	f := &peopleFixture{
		links: l, users: u, store: &peopleStore{links: l, users: u, resets: map[string]string{}},
		sessions: &sessions{}, cleared: &cleared{}, publisher: &publisher{},
	}
	r := roles{"t-places/u-maint": "maintainer", "t-places/u-member": "member"}
	policy := authz.NewPolicy(r, r)
	mode := accounts.NewMode(cfg)
	f.invites = accounts.NewInvitesUsecase(cfg, mode, l, f.store, teams{"places": "t-places", "reviews": "t-reviews"}, policy, f.sessions, f.publisher)
	f.people = accounts.NewPeopleUsecase(cfg, mode, l, f.store, u, policy, f.cleared, f.publisher)
	return f
}
