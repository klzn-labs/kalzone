package tests

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/stretchr/testify/require"
)

type issuedToken struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	Prefix    string    `json:"prefix"`
	ExpiresAt time.Time `json:"expires_at"`
}

type whoami struct {
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
	Token *struct {
		Name   string   `json:"name"`
		Prefix string   `json:"prefix"`
		Scopes []string `json:"scopes"`
		Teams  []string `json:"teams"`
		Bypass bool     `json:"bypass"`
	} `json:"token"`
}

func tokenBody(name string, scopes ...string) map[string]any {
	return map[string]any{"name": name, "scopes": scopes, "teams": []string{"places"}}
}

func issue(t *testing.T, c *integration.Client, body map[string]any) issuedToken {
	t.Helper()
	var out issuedToken
	c.MustJSON(t, http.MethodPost, "/api/v1/tokens", body, http.StatusCreated, &out)
	return out
}

func Test_ABearerTokenCallsTheAPIWithoutCSRF(t *testing.T) {
	s, p := setupPlaces(t)
	agent := s.WithToken(t, issue(t, p.bob, tokenBody("claude-local", "read", "config")).Token)

	resp := agent.Do(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", map[string]any{"set_env": map[string]string{"A": "1"}})

	require.Equal(t, http.StatusCreated, resp.Status, resp.Body)
}

func Test_ABearerTokenIsRefusedOnPages(t *testing.T) {
	s, p := setupPlaces(t)
	agent := s.WithToken(t, issue(t, p.bob, tokenBody("claude-local", "read")).Token)

	home := agent.Do(t, http.MethodGet, "/", nil)
	grafana := agent.Do(t, http.MethodGet, "/grafana/", nil)

	require.Equal(t, http.StatusUnauthorized, home.Status, home.Body)
	require.Contains(t, home.Body, "err_unauthenticated")
	require.Equal(t, http.StatusUnauthorized, grafana.Status, grafana.Body)
}

func Test_InvalidExpiredAndRevokedTokensGetTheirCodes(t *testing.T) {
	s, p := setupPlaces(t)
	revoked := issue(t, p.bob, tokenBody("revoked", "read"))
	expiring := issue(t, p.bob, map[string]any{"name": "short", "scopes": []string{"read"}, "teams": []string{"places"}, "expires_in_days": 1})
	p.bob.MustJSON(t, http.MethodDelete, "/api/v1/tokens/"+revoked.ID, nil, http.StatusNoContent, nil)

	garbage := s.WithToken(t, "plat_nope").Do(t, http.MethodGet, "/api/v1/whoami", nil)
	gone := s.WithToken(t, revoked.Token).Do(t, http.MethodGet, "/api/v1/whoami", nil)
	clock.WithTime(time.Now().Add(25 * time.Hour))
	t.Cleanup(func() { clock.Clock = nil })
	late := s.WithToken(t, expiring.Token).Do(t, http.MethodGet, "/api/v1/whoami", nil)

	require.Equal(t, http.StatusUnauthorized, garbage.Status)
	require.Contains(t, garbage.Body, "err_token_invalid")
	require.Equal(t, http.StatusUnauthorized, gone.Status)
	require.Contains(t, gone.Body, "err_token_revoked")
	require.Equal(t, http.StatusUnauthorized, late.Status)
	require.Contains(t, late.Body, "err_token_expired")
}

func Test_BearerWinsOverTheSessionCookie(t *testing.T) {
	_, p := setupPlaces(t)
	miaToken := issue(t, p.mia, tokenBody("mia-agent", "read"))
	p.bob.UseToken(miaToken.Token)
	var who whoami

	p.bob.MustJSON(t, http.MethodGet, "/api/v1/whoami", nil, http.StatusOK, &who)

	require.Equal(t, "mia", who.Owner.Login)
	require.Equal(t, "mia-agent", who.Token.Name)
}

func sessionCookie(t *testing.T, s *integration.Suite, c *integration.Client) string {
	t.Helper()
	u, err := url.Parse(s.Server.URL)
	require.NoError(t, err)
	for _, cookie := range c.Cookies(u) {
		if cookie.Name == "platform_session" {
			return cookie.Value
		}
	}
	t.Fatal("no session cookie")
	return ""
}

func Test_MalformedAuthorizationHeaders(t *testing.T) {
	s, p := setupPlaces(t)
	cookie := "platform_session=" + sessionCookie(t, s, p.bob)
	whoamiWith := func(headers map[string]string) integration.Response {
		return s.Raw(t, http.MethodGet, "/api/v1/whoami", nil, headers)
	}

	sessionAsBearer := whoamiWith(map[string]string{"Authorization": "Bearer " + sessionCookie(t, s, p.bob)})
	lowercase := whoamiWith(map[string]string{"Authorization": "bearer plat_nope"})
	empty := whoamiWith(map[string]string{"Authorization": "Bearer "})
	basic := whoamiWith(map[string]string{"Authorization": "Basic Zm9vOmJhcg==", "Cookie": cookie})
	bearerAndCookie := whoamiWith(map[string]string{"Authorization": "Bearer plat_nope", "Cookie": cookie})

	require.Contains(t, sessionAsBearer.Body, "err_token_invalid")
	require.Contains(t, lowercase.Body, "err_token_invalid")
	require.Equal(t, http.StatusUnauthorized, empty.Status)
	require.Equal(t, http.StatusOK, basic.Status, basic.Body)
	require.Contains(t, bearerAndCookie.Body, "err_token_invalid")
}

func Test_AGetNeedsTheReadScopeExceptWhoami(t *testing.T) {
	s, p := setupPlaces(t)
	agent := s.WithToken(t, issue(t, p.bob, tokenBody("deployer", "deploy")).Token)

	services := agent.Do(t, http.MethodGet, "/api/v1/services", nil)
	who := agent.Do(t, http.MethodGet, "/api/v1/whoami", nil)

	require.Equal(t, http.StatusForbidden, services.Status)
	require.Contains(t, services.Body, `"code":"err_missing_scope"`)
	require.Contains(t, services.Body, `"required_scope":"read"`)
	require.Equal(t, http.StatusOK, who.Status, who.Body)
}
