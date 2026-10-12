package tests

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/adrianozp/platform/internal/integration"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/stretchr/testify/require"
)

func tokenForm(csrf, name string) url.Values {
	return url.Values{"_csrf": {csrf}, "name": {name}, "scopes": {"read", "deploy"}, "teams": {"places"}, "expires_in_days": {"30"}}
}

func Test_TheTokensPageShowsTheSecretOnce(t *testing.T) {
	_, p := setupPlaces(t)

	created := p.bob.Form(t, "/tokens", tokenForm(p.bob.CSRFToken, "laptop"))
	later := p.bob.Do(t, http.MethodGet, "/tokens", nil)
	secret := secretFrom(t, created.Body)

	require.Equal(t, http.StatusOK, created.Status, created.Body)
	require.Equal(t, "no-store", created.CacheControl)
	require.Regexp(t, `^plat_[0-9A-Za-z]{43}$`, secret)
	require.Contains(t, later.Body, "laptop")
	require.Contains(t, later.Body, secret[:13])
	require.False(t, strings.Contains(later.Body, secret))
}

func secretFrom(t *testing.T, body string) string {
	t.Helper()
	marker := `<code class="mono">`
	start := strings.Index(body, marker)
	require.GreaterOrEqual(t, start, 0, body)
	return body[start+len(marker) : start+len(marker)+48]
}

func Test_OnlyAdminsSeeBypassAndEveryonesTokens(t *testing.T) {
	_, p := setupPlaces(t)
	p.bob.Form(t, "/tokens", tokenForm(p.bob.CSRFToken, "bob-laptop"))

	member := p.bob.Do(t, http.MethodGet, "/tokens", nil)
	admin := p.root.Do(t, http.MethodGet, "/tokens", nil)

	require.NotContains(t, member.Body, `name="bypass"`)
	require.Contains(t, admin.Body, `name="bypass"`)
	require.Contains(t, admin.Body, "bob-laptop")
	require.Contains(t, admin.Body, "@bob")
}

func Test_RevokingFromThePage(t *testing.T) {
	s, p := setupPlaces(t)
	issued := issue(t, p.bob, tokenBody("to-revoke", "read"))

	resp := p.bob.Form(t, "/tokens/"+issued.ID+"/revoke", url.Values{"_csrf": {p.bob.CSRFToken}})
	after := s.WithToken(t, issued.Token).Do(t, http.MethodGet, "/api/v1/whoami", nil)

	require.Equal(t, http.StatusNoContent, resp.Status, resp.Body)
	require.Contains(t, after.Body, "err_token_revoked")
}

func Test_OnlyUnrevokedTokensOfferRevoke(t *testing.T) {
	_, p := setupPlaces(t)
	live := issue(t, p.bob, tokenBody("live", "read"))
	gone := issue(t, p.bob, tokenBody("gone", "read"))
	p.bob.MustJSON(t, http.MethodDelete, "/api/v1/tokens/"+gone.ID, nil, http.StatusNoContent, nil)
	expiring := issueInThePast(t, p.bob)

	page := p.bob.Do(t, http.MethodGet, "/tokens", nil)

	require.Contains(t, page.Body, "/tokens/"+live.ID+"/revoke")
	require.Contains(t, page.Body, "/tokens/"+expiring.ID+"/revoke")
	require.NotContains(t, page.Body, "/tokens/"+gone.ID+"/revoke")
	freed := p.bob.Form(t, "/tokens/"+expiring.ID+"/revoke", url.Values{"_csrf": {p.bob.CSRFToken}})
	require.Equal(t, http.StatusNoContent, freed.Status, freed.Body)
}

func issueInThePast(t *testing.T, c *integration.Client) issuedToken {
	t.Helper()
	clock.WithTime(time.Now().Add(-48 * time.Hour))
	defer func() { clock.Clock = nil }()
	return issue(t, c, map[string]any{"name": "short", "scopes": []string{"read"}, "teams": []string{"places"}, "expires_in_days": 1})
}

func Test_TheAuditPageSaysViaWhichToken(t *testing.T) {
	s, p := setupPlaces(t)
	issued := issue(t, p.bob, tokenBody("claude-local", "read", "config"))
	s.WithToken(t, issued.Token).MustJSON(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", map[string]any{"set_env": map[string]string{"A": "1"}}, http.StatusCreated, nil)

	page := p.root.Do(t, http.MethodGet, "/audit", nil)
	filtered := p.root.Do(t, http.MethodGet, "/audit?token="+issued.ID, nil)

	require.Contains(t, page.Body, "via")
	require.Contains(t, page.Body, "claude-local")
	require.Contains(t, filtered.Body, "changed the config of")
	require.NotContains(t, filtered.Body, "created team")
}
