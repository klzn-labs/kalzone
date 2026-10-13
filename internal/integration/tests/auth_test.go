package tests

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/adrianozp/platform/internal/integration"
	"github.com/stretchr/testify/require"
)

var publicRoutes = map[string]bool{
	"GET /healthz":              true,
	"GET /metrics":              true,
	"GET /auth/github":          true,
	"GET /auth/github/callback": true,
}

var param = regexp.MustCompile(`:[a-z]+`)

func Test_SignInMarksConfiguredAdmins(t *testing.T) {
	suite := integration.NewSuite(t)
	var me struct {
		Login string `json:"login"`
		Admin bool   `json:"admin"`
	}

	suite.SignIn(t, "root").MustJSON(t, http.MethodGet, "/api/v1/me", nil, http.StatusOK, &me)
	require.Equal(t, "root", me.Login)
	require.True(t, me.Admin)

	suite.SignIn(t, "ana").MustJSON(t, http.MethodGet, "/api/v1/me", nil, http.StatusOK, &me)
	require.Equal(t, "ana", me.Login)
	require.False(t, me.Admin)
}

func Test_CallbackRejectsAForgedState(t *testing.T) {
	suite := integration.NewSuite(t)
	suite.GitHub.WillSignIn("code", fakeUser())
	client := suite.Anonymous(t)
	client.Do(t, http.MethodGet, "/auth/github", nil)

	resp := client.Do(t, http.MethodGet, "/auth/github/callback?code=code&state=forged", nil)

	require.Equal(t, "/login?error=state", resp.Location)
	require.Equal(t, http.StatusUnauthorized, client.Do(t, http.MethodGet, "/api/v1/me", nil).Status)
}

func Test_CallbackWithABadCodeDoesNotSignIn(t *testing.T) {
	suite := integration.NewSuite(t)
	client := suite.Anonymous(t)
	start := client.Do(t, http.MethodGet, "/auth/github", nil)
	state := stateOf(t, start.Location)

	resp := client.Do(t, http.MethodGet, "/auth/github/callback?code=unknown&state="+state, nil)

	require.Equal(t, "/login?error=github", resp.Location)
}

func Test_LogoutEndsTheSession(t *testing.T) {
	suite := integration.NewSuite(t)
	client := suite.SignIn(t, "ana")

	client.Do(t, http.MethodPost, "/logout", nil)

	require.Equal(t, http.StatusUnauthorized, client.Do(t, http.MethodGet, "/api/v1/me", nil).Status)
}

func Test_AnUnauthenticatedCallLearnsHowToSignIn(t *testing.T) {
	suite := integration.NewSuite(t)

	var body struct {
		Error struct {
			Hint string `json:"hint"`
		} `json:"error"`
	}

	suite.Anonymous(t).MustJSON(t, http.MethodGet, "/api/v1/services", nil, http.StatusUnauthorized, &body)

	require.Equal(t, "send Authorization: Bearer <token>, or sign in to the website", body.Error.Hint)
}

func Test_EveryAPIRouteNeedsASession(t *testing.T) {
	suite := integration.NewSuite(t)
	anonymous := suite.Anonymous(t)

	for _, route := range suite.Routes() {
		key := route.Method + " " + route.Path
		if publicRoutes[key] || !integration.IsAPI(route.Path) {
			continue
		}
		resp := anonymous.Do(t, route.Method, param.ReplaceAllString(route.Path, "x"), nil)
		require.Equal(t, http.StatusUnauthorized, resp.Status, key)
	}
}

func Test_StateChangesNeedTheCSRFToken(t *testing.T) {
	suite := integration.NewSuite(t)
	root := suite.SignIn(t, "root")
	token := root.CSRFToken

	root.CSRFToken = ""
	missing := root.Do(t, http.MethodPost, "/api/v1/teams", map[string]string{"slug": "places", "name": "Places"})
	root.CSRFToken = "wrong"
	wrong := root.Do(t, http.MethodPost, "/api/v1/teams", map[string]string{"slug": "places", "name": "Places"})
	root.CSRFToken = token
	right := root.Do(t, http.MethodPost, "/api/v1/teams", map[string]string{"slug": "places", "name": "Places"})

	require.Equal(t, http.StatusForbidden, missing.Status)
	require.Contains(t, missing.Body, "err_invalid_csrf_token")
	require.Equal(t, http.StatusForbidden, wrong.Status)
	require.Equal(t, http.StatusCreated, right.Status, right.Body)
}
