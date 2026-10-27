package tests

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_PagesRedirectAnonymousVisitorsToLogin(t *testing.T) {
	suite := setupSuiteOnly(t)
	anonymous := suite.Anonymous(t)

	resp := anonymous.Do(t, http.MethodGet, "/services/places-api", nil)
	login := anonymous.Do(t, http.MethodGet, "/login", nil)

	require.Equal(t, http.StatusSeeOther, resp.Status)
	require.Equal(t, "/login?next=%2Fservices%2Fplaces-api", resp.Location)
	require.Equal(t, http.StatusOK, login.Status)
	require.Contains(t, login.Body, "Continue with GitHub")
}

var pages = map[string]string{
	"/":                             "Places API",
	"/services":                     "places-api",
	"/services?filter=mine":         "places-api",
	"/services/new":                 "Register a service",
	"/services/places-api":          "Danger zone",
	"/services/places-api/env/dev":  "LOG_LEVEL",
	"/services/places-api/releases": "Workflow snippet",
	"/teams":                        "Places",
	"/teams/places":                 "@bob",
	"/audit":                        "changed the config of",
	"/audit?team=places":            "created service",
	"/people":                       "@bob",
}

func Test_EveryPageRenders(t *testing.T) {
	_, p := setupPlaces(t)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", map[string]any{"set_env": map[string]string{"LOG_LEVEL": "debug"}}, http.StatusCreated, nil)

	for path, want := range pages {
		resp := p.mia.Do(t, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, resp.Status, path)
		require.Contains(t, resp.Body, want, path)
	}
}

func Test_UnknownServiceShowsAFriendly404(t *testing.T) {
	_, p := setupPlaces(t)

	resp := p.bob.Do(t, http.MethodGet, "/services/ghost", nil)

	require.Equal(t, http.StatusNotFound, resp.Status)
	require.Contains(t, resp.Body, "couldn")
}

func Test_MembersSeeProdReadOnly(t *testing.T) {
	_, p := setupPlaces(t)

	dev := p.bob.Do(t, http.MethodGet, "/services/places-api/env/dev", nil)
	prod := p.bob.Do(t, http.MethodGet, "/services/places-api/env/prod", nil)

	require.Contains(t, dev.Body, "Save secret")
	require.NotContains(t, prod.Body, "Save secret")
	require.Contains(t, prod.Body, "is protected")
}

func Test_FormsChangeConfigAndNeverEchoSecrets(t *testing.T) {
	_, p := setupPlaces(t)
	base := "/services/places-api/env/dev"

	variable := p.bob.Form(t, base+"/variables", url.Values{"_csrf": {p.bob.CSRFToken}, "name": {"LOG_LEVEL"}, "value": {"warn"}})
	secret := p.bob.Form(t, base+"/secrets", url.Values{"_csrf": {p.bob.CSRFToken}, "name": {"API_TOKEN"}, "value": {secretValue}})
	page := p.bob.Do(t, http.MethodGet, base, nil)

	require.Equal(t, http.StatusNoContent, variable.Status, variable.Body)
	require.Equal(t, http.StatusNoContent, secret.Status, secret.Body)
	require.Contains(t, page.Body, "API_TOKEN")
	require.Contains(t, page.Body, "warn")
	require.NotContains(t, page.Body, secretValue)
}

func Test_FormErrorsComeBackAsAnInlineAlert(t *testing.T) {
	_, p := setupPlaces(t)

	resp := p.bob.Form(t, "/services/places-api/env/prod/variables", url.Values{"_csrf": {p.bob.CSRFToken}, "name": {"X"}, "value": {"1"}})
	invalid := p.bob.Form(t, "/services/places-api/env/dev/variables", url.Values{"_csrf": {p.bob.CSRFToken}, "name": {"lower"}, "value": {"1"}})

	require.Equal(t, http.StatusUnprocessableEntity, resp.Status)
	require.Contains(t, resp.Body, "not allowed")
	require.Equal(t, http.StatusUnprocessableEntity, invalid.Status)
	require.True(t, strings.Contains(invalid.Body, "UPPER_SNAKE_CASE"), invalid.Body)
}

func Test_FormsWithoutTheCSRFTokenAreRefused(t *testing.T) {
	_, p := setupPlaces(t)

	resp := p.bob.Form(t, "/services/places-api/env/dev/variables", url.Values{"name": {"A"}, "value": {"1"}})

	require.Equal(t, http.StatusForbidden, resp.Status)
}

func Test_GrafanaIsBehindThePlatformsSignIn(t *testing.T) {
	s, p := setupPlaces(t)

	anonymous := s.Anonymous(t).Do(t, http.MethodGet, "/grafana/api/user", nil)
	spoofed := s.Raw(t, http.MethodGet, "/grafana/api/user", nil, map[string]string{"X-WEBAUTH-USER": "root"})
	member := p.bob.Do(t, http.MethodGet, "/grafana/api/user", nil)
	admin := p.root.Do(t, http.MethodGet, "/grafana/api/user", nil)

	require.Equal(t, http.StatusSeeOther, anonymous.Status)
	require.Contains(t, spoofed.Body, "Continue with GitHub")
	require.JSONEq(t, `{"path":"/grafana/api/user","user":"bob","role":""}`, member.Body)
	require.JSONEq(t, `{"path":"/grafana/api/user","user":"root","role":""}`, admin.Body)
}

func Test_GrafanaKeysAreNotServedWithoutASigningKey(t *testing.T) {
	s, _ := setupPlaces(t)

	keys := s.Anonymous(t).Do(t, http.MethodGet, "/internal/grafana/keys.json", nil)

	require.Equal(t, http.StatusNotFound, keys.Status)
}

func Test_TheCatalogGroupsServicesUnderTheirTeams(t *testing.T) {
	_, p := setupPlaces(t)
	createReviews(t, p)
	p.root.MustJSON(t, http.MethodPost, "/api/v1/services", reviewsService(), http.StatusCreated, nil)

	body := p.root.Do(t, http.MethodGet, "/services", nil).Body
	order := []int{
		strings.Index(body, `data-team="places"`), strings.Index(body, `href="/services/places-api"`),
		strings.Index(body, `data-team="reviews"`), strings.Index(body, `href="/services/reviews-api"`),
	}

	require.NotContains(t, order, -1)
	require.IsIncreasing(t, order)
}

func Test_TeamPageSuggestsOnlyPeopleWhoShareATeam(t *testing.T) {
	_, p := setupPlaces(t)

	page := p.mia.Do(t, http.MethodGet, "/teams/places", nil)
	everyone := p.root.Do(t, http.MethodGet, "/teams/places", nil)

	require.Contains(t, page.Body, `list="member-logins"`)
	require.Contains(t, page.Body, `<option value="bob">`)
	require.NotContains(t, page.Body, `<option value="eve">`)
	require.Contains(t, everyone.Body, `<option value="eve">`)
}

func Test_EveryPageCanSwitchTheme(t *testing.T) {
	_, p := setupPlaces(t)

	page := p.mia.Do(t, http.MethodGet, "/services", nil)
	script := p.mia.Do(t, http.MethodGet, "/static/theme.js", nil)

	require.Contains(t, page.Body, `<script src="/static/theme.js"></script>`)
	require.Contains(t, page.Body, "data-theme-toggle")
	require.Equal(t, http.StatusOK, script.Status)
	require.Contains(t, script.Body, "kalzone-theme")
}
