package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

var placesReads = []string{
	"/api/v1/services/places-api", "/api/v1/config/places-api/dev", "/api/v1/config/places-api/dev/revisions",
	"/api/v1/releases?service=places-api", "/api/v1/deployments?service=places-api&env=dev", "/api/v1/teams/places",
	"/api/v1/teams/places/members", "/api/v1/users/bob",
}

var placesPages = []string{
	"/services/places-api", "/services/places-api/env/dev", "/services/places-api/releases", "/teams/places",
}

func Test_SomeoneOnNoTeamSeesNothingOfOtherTeams(t *testing.T) {
	_, p := setupPlaces(t)
	var services serviceList
	var teams, audit struct {
		Items []map[string]any `json:"items"`
	}

	p.eve.MustJSON(t, http.MethodGet, "/api/v1/services", nil, http.StatusOK, &services)
	p.eve.MustJSON(t, http.MethodGet, "/api/v1/teams", nil, http.StatusOK, &teams)
	p.eve.MustJSON(t, http.MethodGet, "/api/v1/audit", nil, http.StatusOK, &audit)

	require.Empty(t, services.Services)
	require.Empty(t, teams.Items)
	require.Empty(t, audit.Items)
	for _, path := range placesReads {
		require.Equal(t, http.StatusNotFound, p.eve.Do(t, http.MethodGet, path, nil).Status, path)
	}
	for _, path := range placesPages {
		require.Equal(t, http.StatusNotFound, p.eve.Do(t, http.MethodGet, path, nil).Status, path)
	}
}

func Test_ANewcomerIsToldToAskForATeam(t *testing.T) {
	_, p := setupPlaces(t)

	newcomer := p.eve.Do(t, http.MethodGet, "/", nil)
	member := p.bob.Do(t, http.MethodGet, "/", nil)

	require.Equal(t, http.StatusOK, newcomer.Status, newcomer.Body)
	require.Contains(t, newcomer.Body, "You're not on a team yet")
	require.Contains(t, newcomer.Body, "@eve")
	require.NotContains(t, newcomer.Body, "Places API")
	require.NotContains(t, member.Body, "You're not on a team yet")
}

func Test_ATokenSeesOnlyTeamsItsOwnerBelongsTo(t *testing.T) {
	s, p := setupPlaces(t)
	createReviews(t, p)
	p.root.MustJSON(t, http.MethodPost, "/api/v1/services", reviewsService(), http.StatusCreated, nil)
	body := map[string]any{"name": "both", "scopes": []string{"read"}, "teams": []string{"places", "reviews"}}
	token := issue(t, p.bob, body).Token
	var listed serviceList

	s.WithToken(t, token).MustJSON(t, http.MethodGet, "/api/v1/services", nil, http.StatusOK, &listed)
	session := mcpSession(t, s, token)
	text, failed := callTool(t, session, "getService", map[string]any{"service": "reviews-api"})
	own, ownFailed := callTool(t, session, "getService", map[string]any{"service": "places-api"})

	require.Len(t, listed.Services, 1)
	require.Equal(t, "places-api", listed.Services[0].Slug)
	require.False(t, ownFailed, own)
	require.True(t, failed, text)
	require.Contains(t, text, "not found")
}

func Test_AnAdminSeesEveryTeam(t *testing.T) {
	_, p := setupPlaces(t)
	createReviews(t, p)
	p.root.MustJSON(t, http.MethodPost, "/api/v1/services", reviewsService(), http.StatusCreated, nil)
	var listed serviceList

	p.root.MustJSON(t, http.MethodGet, "/api/v1/services", nil, http.StatusOK, &listed)

	require.Len(t, listed.Services, 2)
}
