package tests

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/usecases/accounts"
	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

const (
	ownerLogin       = "owner"
	ownerPassword    = "owner-first-password"
	ownerNewPassword = "owner-second-password"
	anaPassword      = "ana-first-password"
	anaNewPassword   = "ana-second-password"
	wrongPassword    = "not-the-right-password"
	linkGone         = "This link is no longer valid."
	githubModeHint   = "this Kalzone signs people in with GitHub; add them to the team after they sign in"
)

var (
	agentSecret = regexp.MustCompile(`id="agent-token" type="text" class="mono" value="(plat_[0-9A-Za-z]+)"`)
	joinLink    = regexp.MustCompile(`http://platform\.test/join/[^\s<"&]+`)
)

type owned struct {
	s     *integration.Suite
	owner *integration.Client
	agent string
	link  string
	done  integration.Response
}

type createdInvite struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

func passwordSuite(t *testing.T) *integration.Suite {
	t.Helper()
	return integration.NewSuite(t, integration.WithPasswordMode)
}

func setUpOwner(t *testing.T) owned {
	t.Helper()
	s := passwordSuite(t)
	return ownedBy(t, s, s.SetupLink(t))
}

func ownedBy(t *testing.T, s *integration.Suite, link string) owned {
	t.Helper()
	owner := s.Anonymous(t)
	form := credentials(ownerLogin, ownerPassword)
	form.Set("bypass", "on")
	done := owner.Form(t, joinPath(t, link), form)
	require.Equal(t, http.StatusOK, done.Status, done.Body)
	owner.LoadCSRF(t)
	found := agentSecret.FindStringSubmatch(done.Body)
	require.Len(t, found, 2, done.Body)
	return owned{s: s, owner: owner, agent: found[1], link: link, done: done}
}

func credentials(username, password string) url.Values {
	return url.Values{"username": {username}, "password": {password}, "confirm": {password}}
}

func joinPath(t *testing.T, link string) string {
	t.Helper()
	parsed, err := url.Parse(link)
	require.NoError(t, err)
	return parsed.Path
}

func linkIn(t *testing.T, body string) string {
	t.Helper()
	link := joinLink.FindString(body)
	require.NotEmpty(t, link, body)
	return link
}

func createInvite(t *testing.T, admin *integration.Client, role string) createdInvite {
	t.Helper()
	var created createdInvite
	admin.MustJSON(t, http.MethodPost, "/api/v1/teams/default/invites", map[string]string{"role": role}, http.StatusCreated, &created)
	return created
}

func join(t *testing.T, s *integration.Suite, link, username, password string) *integration.Client {
	t.Helper()
	c := s.Anonymous(t)
	joined := c.Form(t, joinPath(t, link), credentials(username, password))
	require.Equal(t, http.StatusSeeOther, joined.Status, joined.Body)
	require.Equal(t, "/", joined.Location)
	c.LoadCSRF(t)
	return c
}

func roleOf(t *testing.T, c *integration.Client, login string) string {
	t.Helper()
	var members struct {
		Items []struct {
			Login string `json:"login"`
			Role  string `json:"role"`
		} `json:"items"`
	}
	c.MustJSON(t, http.MethodGet, "/api/v1/teams/default/members", nil, http.StatusOK, &members)
	for _, m := range members.Items {
		if m.Login == login {
			return m.Role
		}
	}
	return ""
}

func issueReset(t *testing.T, admin *integration.Client, login string) string {
	t.Helper()
	page := admin.Form(t, "/people/"+login+"/reset", url.Values{"_csrf": {admin.CSRFToken}})
	require.Equal(t, http.StatusOK, page.Status, page.Body)
	require.Equal(t, "no-store", page.CacheControl)
	return linkIn(t, page.Body)
}

func useReset(t *testing.T, s *integration.Suite, link, password string) {
	t.Helper()
	done := s.Anonymous(t).Form(t, joinPath(t, link), url.Values{"password": {password}, "confirm": {password}})
	require.Equal(t, http.StatusOK, done.Status, done.Body)
	require.Contains(t, done.Body, "Your password is set")
}

func removePerson(t *testing.T, admin *integration.Client, login string) {
	t.Helper()
	removed := admin.Form(t, "/people/"+login+"/remove", url.Values{"_csrf": {admin.CSRFToken}})
	require.Equal(t, http.StatusNoContent, removed.Status, removed.Body)
}

func attempt(t *testing.T, s *integration.Suite, username, password, ip string) integration.Response {
	t.Helper()
	form := url.Values{"username": {username}, "password": {password}}
	return s.Anonymous(t).FormWith(t, "/login", form, map[string]string{"X-Forwarded-For": ip})
}

func problemOf(t *testing.T, answer integration.Response) string {
	t.Helper()
	require.Equal(t, http.StatusSeeOther, answer.Status, answer.Body)
	parsed, err := url.Parse(answer.Location)
	require.NoError(t, err)
	return parsed.Query().Get("error")
}

func visit(t *testing.T, s *integration.Suite, link string) integration.Response {
	t.Helper()
	return s.Anonymous(t).Do(t, http.MethodGet, joinPath(t, link), nil)
}

func backdated(t *testing.T, by time.Duration, issue func() string) string {
	t.Helper()
	clock.WithTime(time.Now().Add(-by))
	defer func() { clock.Clock = nil }()
	return issue()
}

func Test_SetupCreatesOwnerTeamAndWorkingToken(t *testing.T) {
	o := setUpOwner(t)
	var agent whoami
	var self struct {
		Admin bool `json:"admin"`
	}

	o.s.WithToken(t, o.agent).MustJSON(t, http.MethodGet, "/api/v1/whoami", nil, http.StatusOK, &agent)
	o.owner.MustJSON(t, http.MethodGet, "/api/v1/me", nil, http.StatusOK, &self)
	tools := listTools(t, mcpSession(t, o.s, o.agent))

	require.Equal(t, "no-store", o.done.CacheControl)
	require.Contains(t, o.done.Body, "claude mcp add --transport http kalzone http://platform.test/mcp --header")
	require.Equal(t, ownerLogin, agent.Owner.Login)
	require.Equal(t, "agent", agent.Token.Name)
	require.Equal(t, []string{"default"}, agent.Token.Teams)
	require.True(t, agent.Token.Bypass)
	require.True(t, self.Admin)
	require.Equal(t, "maintainer", roleOf(t, o.owner, ownerLogin))
	requireTools(t, tools, "getWhoami", "createInvite", "listInvites", "revokeInvite")
}

func requireTools(t *testing.T, tools map[string]*mcp.Tool, names ...string) {
	t.Helper()
	for _, name := range names {
		require.Contains(t, tools, name)
	}
}

func Test_SetupLinkWorksOnce(t *testing.T) {
	o := setUpOwner(t)
	again := o.s.Anonymous(t)

	page := again.Do(t, http.MethodGet, joinPath(t, o.link), nil)
	resubmitted := again.Form(t, joinPath(t, o.link), credentials("intruder", ownerPassword))

	require.Equal(t, http.StatusNotFound, page.Status)
	require.Contains(t, page.Body, linkGone)
	require.Equal(t, http.StatusNotFound, resubmitted.Status)
	require.Contains(t, resubmitted.Body, linkGone)
	require.Empty(t, again.Cookies(mustURL(t, o.s.Server.URL)))
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed
}

func Test_NoSecondSetupLinkOnceOwned(t *testing.T) {
	o := setUpOwner(t)

	_, refused := o.s.Accounts.NewSetupLink(context.Background())
	restarted := o.s.Accounts.EnsureSetupLink(context.Background())

	require.Equal(t, apperror.CodeConflict, apperror.CodeOf(refused))
	require.NoError(t, restarted)
	require.Equal(t, 1, strings.Count(o.s.Logs.String(), "WARN setup:"))
}

func Test_DeadLinksShowOnePage(t *testing.T) {
	s := passwordSuite(t)
	pages, live := deadSetupPages(t, s)
	o := ownedBy(t, s, live)
	maps.Copy(pages, deadInvitePages(t, o))
	maps.Copy(pages, deadResetPages(t, o))
	pages["used setup"] = visit(t, s, live)
	reference := s.Anonymous(t).Do(t, http.MethodGet, "/join/never-issued", nil)

	require.Equal(t, http.StatusNotFound, reference.Status)
	require.Contains(t, reference.Body, linkGone)
	for name, page := range pages {
		require.Equal(t, http.StatusNotFound, page.Status, name)
		require.Equal(t, reference.Body, page.Body, name)
	}
}

func deadSetupPages(t *testing.T, s *integration.Suite) (map[string]integration.Response, string) {
	t.Helper()
	startup := s.SetupLink(t)
	expired := backdated(t, 25*time.Hour, func() string { return newSetupLink(t, s) })
	pages := map[string]integration.Response{"revoked setup": visit(t, s, startup), "expired setup": visit(t, s, expired)}
	return pages, newSetupLink(t, s)
}

func newSetupLink(t *testing.T, s *integration.Suite) string {
	t.Helper()
	link, err := s.Accounts.NewSetupLink(context.Background())
	require.NoError(t, err)
	return link
}

func deadInvitePages(t *testing.T, o owned) map[string]integration.Response {
	t.Helper()
	used := createInvite(t, o.owner, "member")
	join(t, o.s, used.URL, "ana", anaPassword)
	revoked := createInvite(t, o.owner, "member")
	o.owner.MustJSON(t, http.MethodDelete, "/api/v1/teams/default/invites/"+revoked.ID, nil, http.StatusNoContent, nil)
	expired := backdated(t, 8*24*time.Hour, func() string { return createInvite(t, o.owner, "member").URL })
	return map[string]integration.Response{
		"used invite": visit(t, o.s, used.URL), "revoked invite": visit(t, o.s, revoked.URL), "expired invite": visit(t, o.s, expired),
	}
}

func deadResetPages(t *testing.T, o owned) map[string]integration.Response {
	t.Helper()
	used := issueReset(t, o.owner, "ana")
	useReset(t, o.s, used, anaNewPassword)
	replaced := issueReset(t, o.owner, "ana")
	issueReset(t, o.owner, "ana")
	expired := backdated(t, 25*time.Hour, func() string { return issueReset(t, o.owner, "ana") })
	return map[string]integration.Response{
		"used reset": visit(t, o.s, used), "replaced reset": visit(t, o.s, replaced), "expired reset": visit(t, o.s, expired),
	}
}

func Test_ConcurrentUsesOfOneLinkCreateOneUser(t *testing.T) {
	o := setUpOwner(t)
	link := joinPath(t, createInvite(t, o.owner, "member").URL)

	statuses := race(t, o.s, link, 2)

	require.ElementsMatch(t, []int{http.StatusSeeOther, http.StatusNotFound}, statuses)
	joined := []string{roleOf(t, o.owner, "racer-0"), roleOf(t, o.owner, "racer-1")}
	require.ElementsMatch(t, []string{"member", ""}, joined)
}

func race(t *testing.T, s *integration.Suite, path string, n int) []int {
	t.Helper()
	clients, answers, errs := make([]*integration.Client, n), make([]integration.Response, n), make([]error, n)
	for i := range clients {
		clients[i] = s.Anonymous(t)
	}
	var wg sync.WaitGroup
	for i, c := range clients {
		wg.Go(func() {
			answers[i], errs[i] = c.TryForm(path, credentials(fmt.Sprintf("racer-%d", i), anaPassword), nil)
		})
	}
	wg.Wait()
	statuses := make([]int, n)
	for i := range answers {
		require.NoError(t, errs[i])
		statuses[i] = answers[i].Status
	}
	return statuses
}

func Test_SignInFailuresLookTheSame(t *testing.T) {
	o := setUpOwner(t)
	join(t, o.s, createInvite(t, o.owner, "member").URL, "ana", anaPassword)
	removePerson(t, o.owner, "ana")

	answers := map[string]integration.Response{
		"wrong password":     attempt(t, o.s, ownerLogin, wrongPassword, "10.1.0.1"),
		"unknown user":       attempt(t, o.s, "nobody", wrongPassword, "10.1.0.2"),
		"disabled user":      attempt(t, o.s, "ana", anaPassword, "10.1.0.3"),
		"malformed username": attempt(t, o.s, "Not A Login!", wrongPassword, "10.1.0.4"),
	}
	page := o.s.Anonymous(t).Do(t, http.MethodGet, "/login?error=password", nil)

	for name, answer := range answers {
		require.Equal(t, "password", problemOf(t, answer), name)
	}
	require.Contains(t, page.Body, "Wrong username or password.")
}

func Test_EleventhFailureIsThrottled(t *testing.T) {
	o := setUpOwner(t)
	for range 10 {
		require.Equal(t, "password", problemOf(t, attempt(t, o.s, ownerLogin, wrongPassword, "10.2.0.1")))
	}

	throttled := attempt(t, o.s, ownerLogin, ownerPassword, "10.2.0.1")
	stranger := attempt(t, o.s, "someone-else", wrongPassword, "10.2.0.1")
	elsewhere := attempt(t, o.s, ownerLogin, ownerPassword, "10.2.0.2")
	page := o.s.Anonymous(t).Do(t, http.MethodGet, throttled.Location, nil)

	require.Equal(t, "throttled", problemOf(t, throttled))
	require.Equal(t, "throttled", problemOf(t, stranger))
	require.Contains(t, page.Body, "Too many attempts, try again later.")
	require.Equal(t, http.StatusSeeOther, elsewhere.Status)
	require.Equal(t, "/", elsewhere.Location)
}

func Test_CrossSiteSignInIsRefused(t *testing.T) {
	o := setUpOwner(t)
	link := joinPath(t, createInvite(t, o.owner, "member").URL)
	c := o.s.Anonymous(t)

	foreign := c.FormWith(t, "/login", credentials(ownerLogin, ownerPassword), map[string]string{"Origin": "https://evil.example"})
	crossSite := c.FormWith(t, link, credentials("ana", anaPassword), map[string]string{"Sec-Fetch-Site": "cross-site"})
	sameOrigin := c.FormWith(t, "/login", credentials(ownerLogin, ownerPassword), map[string]string{"Origin": "http://platform.test", "Sec-Fetch-Site": "same-origin"})

	require.Equal(t, http.StatusForbidden, foreign.Status)
	require.Contains(t, foreign.Body, `href="http://platform.test"`)
	require.Equal(t, http.StatusForbidden, crossSite.Status)
	require.Equal(t, http.StatusSeeOther, sameOrigin.Status)
	require.Equal(t, "/", sameOrigin.Location)
	require.Empty(t, roleOf(t, o.owner, "ana"))
}

func Test_InviteJoinsTeamWithItsRole(t *testing.T) {
	o := setUpOwner(t)
	member := createInvite(t, o.owner, "member")
	page := o.owner.Form(t, "/teams/default/invites", url.Values{"_csrf": {o.owner.CSRFToken}, "role": {"maintainer"}})
	maintainer := linkIn(t, page.Body)

	join(t, o.s, member.URL, "ana", anaPassword)
	join(t, o.s, maintainer, "bea", anaPassword)
	later := o.owner.Do(t, http.MethodGet, "/teams/default", nil)

	require.Equal(t, http.StatusOK, page.Status, page.Body)
	require.Equal(t, "no-store", page.CacheControl)
	require.Equal(t, "member", roleOf(t, o.owner, "ana"))
	require.Equal(t, "maintainer", roleOf(t, o.owner, "bea"))
	require.NotContains(t, later.Body, joinPath(t, maintainer))
}

func Test_InviteRoleCannotBeEdited(t *testing.T) {
	o := setUpOwner(t)
	link := createInvite(t, o.owner, "member").URL
	form := credentials("ana", anaPassword)
	form.Set("role", "maintainer")

	joined := o.s.Anonymous(t).Form(t, joinPath(t, link), form)

	require.Equal(t, http.StatusSeeOther, joined.Status, joined.Body)
	require.Equal(t, "member", roleOf(t, o.owner, "ana"))
}

func Test_InviteAsksASignedInPersonToSignOut(t *testing.T) {
	o := setUpOwner(t)
	link := createInvite(t, o.owner, "member").URL

	page := o.owner.Do(t, http.MethodGet, joinPath(t, link), nil)
	refused := o.owner.Form(t, joinPath(t, link), credentials("ana", anaPassword))
	join(t, o.s, link, "ana", anaPassword)

	require.Equal(t, http.StatusOK, page.Status)
	require.Contains(t, page.Body, "Sign out to accept this invite")
	require.NotEqual(t, http.StatusSeeOther, refused.Status)
	require.Contains(t, refused.Body, "Sign out to accept this invite")
	require.Equal(t, "member", roleOf(t, o.owner, "ana"))
}

func Test_RemovedPersonIsSignedOutEverywhere(t *testing.T) {
	o := setUpOwner(t)
	ana := join(t, o.s, createInvite(t, o.owner, "member").URL, "ana", anaPassword)
	token := issue(t, ana, map[string]any{"name": "laptop", "scopes": []string{"read"}, "teams": []string{"default"}}).Token
	requireSignedIn(t, o.s.WithToken(t, token), ana)

	removePerson(t, o.owner, "ana")
	withToken := o.s.WithToken(t, token).Do(t, http.MethodGet, "/api/v1/whoami", nil)
	withSession := ana.Do(t, http.MethodGet, "/", nil)
	signIn := attempt(t, o.s, "ana", anaPassword, "10.3.0.1")
	reset := o.owner.Form(t, "/people/ana/reset", url.Values{"_csrf": {o.owner.CSRFToken}})

	require.Equal(t, http.StatusUnauthorized, withToken.Status, withToken.Body)
	require.True(t, strings.HasPrefix(withSession.Location, "/login"), withSession.Location)
	require.Equal(t, "password", problemOf(t, signIn))
	require.Equal(t, http.StatusUnprocessableEntity, reset.Status, reset.Body)
	require.NotRegexp(t, joinLink, reset.Body)
}

func Test_LocalLoginInAdminsListIsNotAdmin(t *testing.T) {
	o := setUpOwner(t)
	joined := join(t, o.s, createInvite(t, o.owner, "member").URL, "root", anaPassword)
	again := o.s.SignInWithPassword(t, "root", anaPassword)

	for _, c := range []*integration.Client{joined, again} {
		var me struct {
			Admin bool `json:"admin"`
		}
		c.MustJSON(t, http.MethodGet, "/api/v1/me", nil, http.StatusOK, &me)
		require.False(t, me.Admin)
	}
}

func Test_PasswordModeHidesGitHub(t *testing.T) {
	s := passwordSuite(t)
	c := s.Anonymous(t)

	start := c.Do(t, http.MethodGet, "/auth/github", nil)
	callback := c.Do(t, http.MethodGet, "/auth/github/callback?code=x&state=y", nil)
	login := c.Do(t, http.MethodGet, "/login", nil)

	require.Equal(t, http.StatusNotFound, start.Status)
	require.Equal(t, http.StatusNotFound, callback.Status)
	require.NotContains(t, login.Body, "Continue with GitHub")
	require.NotContains(t, login.Body, "/auth/github")
	require.Contains(t, login.Body, `action="/login"`)
	require.Contains(t, login.Body, "Ask the owner of this Kalzone for an invite link")
}

func Test_GitHubModeHidesPasswords(t *testing.T) {
	s := integration.NewSuite(t)
	root := s.SignIn(t, "root")
	c := s.Anonymous(t)
	login := c.Do(t, http.MethodGet, "/login", nil)

	statuses := map[string]int{
		"POST /login":            c.Form(t, "/login", credentials("root", ownerPassword)).Status,
		"GET /join/x":            c.Do(t, http.MethodGet, "/join/x", nil).Status,
		"POST /join/x":           c.Form(t, "/join/x", credentials("someone", ownerPassword)).Status,
		"GET /account/password":  root.Do(t, http.MethodGet, "/account/password", nil).Status,
		"POST /account/password": root.Form(t, "/account/password", passwordChange(root, ownerPassword, ownerNewPassword)).Status,
	}

	for name, status := range statuses {
		require.Equal(t, http.StatusNotFound, status, name)
	}
	require.Contains(t, login.Body, "Continue with GitHub")
	require.NotContains(t, login.Body, `name="password"`)
	require.NotContains(t, s.Logs.String(), "setup:")
}

func passwordChange(c *integration.Client, current, next string) url.Values {
	return url.Values{"_csrf": {c.CSRFToken}, "current": {current}, "password": {next}, "confirm": {next}}
}

func Test_GitHubModeRefusesInvitesAndResets(t *testing.T) {
	s := integration.NewSuite(t)
	root := s.SignIn(t, "root")
	s.SignIn(t, "bob")
	root.MustJSON(t, http.MethodPost, "/api/v1/teams", map[string]string{"slug": "places", "name": "Places"}, http.StatusCreated, nil)

	invite := root.Do(t, http.MethodPost, "/api/v1/teams/places/invites", map[string]string{"role": "member"})
	reset := root.Form(t, "/people/bob/reset", url.Values{"_csrf": {root.CSRFToken}})
	_, setupErr := s.Accounts.NewSetupLink(context.Background())

	require.Equal(t, http.StatusConflict, invite.Status, invite.Body)
	require.Contains(t, invite.Body, string(apperror.CodeGitHubMode))
	require.Contains(t, invite.Body, githubModeHint)
	require.Equal(t, http.StatusUnprocessableEntity, reset.Status, reset.Body)
	require.Contains(t, reset.Body, "signs people in with GitHub")
	require.Equal(t, apperror.CodeGitHubMode, apperror.CodeOf(setupErr))
}

func Test_RemovedGitHubUserIsRefusedAtTheCallback(t *testing.T) {
	s := integration.NewSuite(t)
	root := s.SignIn(t, "root")
	s.SignIn(t, "bob")
	removePerson(t, root, "bob")

	_, refused := s.TrySignIn(t, "bob")

	require.Equal(t, http.StatusSeeOther, refused.Status, refused.Body)
	require.True(t, strings.HasPrefix(refused.Location, "/login?error="), refused.Location)
}

func Test_NoSecretsInLogsOrAudit(t *testing.T) {
	o, links, pages := everyAccountFlow(t)
	secrets := []string{ownerPassword, ownerNewPassword, anaPassword, anaNewPassword, wrongPassword, "$argon2id$", o.agent}
	setupToken := tokenOf(t, o.link)
	logs := o.s.Logs.String()

	requireNoneIn(t, "logs", logs, append(secrets, links...))
	require.Equal(t, 1, strings.Count(logs, setupToken), logs)
	for _, details := range o.s.AuditDetails(t) {
		requireNoneIn(t, "audit details", details, append(secrets, append(links, setupToken)...))
	}
	for _, page := range pages {
		requireNoneIn(t, "a later page", page, append(secrets, append(links, setupToken)...))
	}
	require.NotEmpty(t, o.s.AuditDetails(t))
}

func everyAccountFlow(t *testing.T) (owned, []string, []string) {
	t.Helper()
	o := setUpOwner(t)
	invite := createInvite(t, o.owner, "member").URL
	join(t, o.s, invite, "ana", anaPassword)
	reset := issueReset(t, o.owner, "ana")
	useReset(t, o.s, reset, anaNewPassword)
	o.s.SignInWithPassword(t, "ana", anaNewPassword)
	require.Equal(t, "password", problemOf(t, attempt(t, o.s, "ana", wrongPassword, "10.4.0.1")))
	changed := o.owner.Form(t, "/account/password", passwordChange(o.owner, ownerPassword, ownerNewPassword))
	require.Equal(t, http.StatusNoContent, changed.Status, changed.Body)
	o.s.SignInWithPassword(t, ownerLogin, ownerNewPassword)
	pages := []string{o.owner.Do(t, http.MethodGet, "/people", nil).Body, o.owner.Do(t, http.MethodGet, "/teams/default", nil).Body}
	return o, []string{tokenOf(t, invite), tokenOf(t, reset)}, pages
}

func tokenOf(t *testing.T, link string) string {
	t.Helper()
	token := strings.TrimPrefix(joinPath(t, link), "/join/")
	require.NotEmpty(t, token)
	return token
}

func requireNoneIn(t *testing.T, where, text string, secrets []string) {
	t.Helper()
	for _, secret := range secrets {
		require.NotContains(t, text, secret, "%s contain a secret", where)
	}
}

func requireSignedIn(t *testing.T, agent, browser *integration.Client) {
	t.Helper()
	agent.MustJSON(t, http.MethodGet, "/api/v1/whoami", nil, http.StatusOK, nil)
	page := browser.Do(t, http.MethodGet, "/", nil)
	require.Equal(t, http.StatusOK, page.Status, page.Location)
}

func requireSignedOut(t *testing.T, browser *integration.Client) {
	t.Helper()
	page := browser.Do(t, http.MethodGet, "/", nil)
	require.True(t, strings.HasPrefix(page.Location, "/login"), page.Location)
	require.Equal(t, http.StatusUnauthorized, browser.Do(t, http.MethodGet, "/api/v1/me", nil).Status)
}

type heldVerifier struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newHeldVerifier() *heldVerifier {
	return &heldVerifier{entered: make(chan struct{}), release: make(chan struct{})}
}

func (v *heldVerifier) verify(hash, password string) bool {
	v.once.Do(func() {
		close(v.entered)
		<-v.release
	})
	return entities.VerifyPassword(hash, password)
}

func (v *heldVerifier) suite(t *testing.T) *integration.Suite {
	t.Helper()
	decorate := fx.Decorate(func(p accounts.PasswordSignIn) accounts.PasswordSignIn { return p.WithVerifier(v.verify) })
	return integration.NewSuiteWith(t, decorate, integration.WithPasswordMode)
}

func Test_ASignInRacingARemovalLeavesNoLiveSession(t *testing.T) {
	held := newHeldVerifier()
	s := held.suite(t)
	o := ownedBy(t, s, s.SetupLink(t))
	join(t, s, createInvite(t, o.owner, "member").URL, "ana", anaPassword)
	ana := s.Anonymous(t)
	signedIn := make(chan integration.Response, 1)
	go func() {
		answer, _ := ana.TryForm("/login", url.Values{"username": {"ana"}, "password": {anaPassword}}, nil)
		signedIn <- answer
	}()

	<-held.entered
	removePerson(t, o.owner, "ana")
	close(held.release)
	answer := <-signedIn

	require.Equal(t, http.StatusSeeOther, answer.Status, answer.Body)
	requireSignedOut(t, ana)
}

func Test_AResetEndsOtherSessionsAndLiftsTheThrottle(t *testing.T) {
	o := setUpOwner(t)
	ana := join(t, o.s, createInvite(t, o.owner, "member").URL, "ana", anaPassword)
	for range 10 {
		attempt(t, o.s, "ana", wrongPassword, "10.5.0.1")
	}
	require.Equal(t, "throttled", problemOf(t, attempt(t, o.s, "ana", anaPassword, "10.5.0.1")))
	link := issueReset(t, o.owner, "ana")

	form := url.Values{"password": {anaNewPassword}, "confirm": {anaNewPassword}}
	done := o.s.Anonymous(t).FormWith(t, joinPath(t, link), form, map[string]string{"X-Forwarded-For": "10.5.0.1"})
	again := attempt(t, o.s, "ana", anaNewPassword, "10.5.0.1")

	require.Contains(t, done.Body, "Your password is set")
	requireSignedOut(t, ana)
	require.Equal(t, http.StatusSeeOther, again.Status)
	require.Equal(t, "/", again.Location)
}
