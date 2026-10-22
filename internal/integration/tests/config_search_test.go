package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type matchView struct {
	Service     string  `json:"service"`
	Environment string  `json:"environment"`
	Revision    int     `json:"revision"`
	Kind        string  `json:"kind"`
	Value       *string `json:"value"`
}

func otherTeamsService(t *testing.T, p people) {
	t.Helper()
	p.root.MustJSON(t, http.MethodPost, "/api/v1/teams", map[string]string{"slug": "money", "name": "Money"}, http.StatusCreated, nil)
	p.root.MustJSON(t, http.MethodPost, "/api/v1/services", map[string]string{
		"slug": "billing", "name": "Billing", "owner_team": "money", "image_repository": "docker.io/adrianozdp/billing",
	}, http.StatusCreated, nil)
	p.root.MustJSON(t, http.MethodPost, "/api/v1/config/billing/dev/revisions", map[string]any{"set_env": map[string]string{"LOG_LEVEL": "warn"}}, http.StatusCreated, nil)
}

func Test_ConfigSearchFindsValuesAndSecretNamesInTheCallersTeams(t *testing.T) {
	s, p := setupPlaces(t)
	otherTeamsService(t, p)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", map[string]any{"set_env": map[string]string{"LOG_LEVEL": "info"}}, http.StatusCreated, nil)
	p.mia.MustJSON(t, http.MethodPut, "/api/v1/config/places-api/prod/secrets/LOG_LEVEL", map[string]string{"value": secretValue}, http.StatusOK, nil)
	reader := agentFor(t, s, p.bob, tokenBody("reader", "read"))
	var found page[matchView]

	resp := reader.MustJSON(t, http.MethodGet, "/api/v1/config/search?key=LOG_LEVEL", nil, http.StatusOK, &found)

	require.Len(t, found.Items, 2)
	require.Equal(t, matchView{Service: "places-api", Environment: "dev", Revision: 1, Kind: "env", Value: &[]string{"info"}[0]}, found.Items[0])
	require.Equal(t, matchView{Service: "places-api", Environment: "prod", Revision: 1, Kind: "secret"}, found.Items[1])
	require.NotContains(t, resp.Body, secretValue)
}

func Test_ConfigSearchNarrowsAndRefusesTeamsTheTokenLacks(t *testing.T) {
	s, p := setupPlaces(t)
	otherTeamsService(t, p)
	reader := agentFor(t, s, p.bob, tokenBody("reader", "read"))
	var money page[matchView]
	var refused, badKey apiError

	p.root.MustJSON(t, http.MethodGet, "/api/v1/config/search?key=LOG_LEVEL&team=money&env=dev", nil, http.StatusOK, &money)
	reader.MustJSON(t, http.MethodGet, "/api/v1/config/search?key=LOG_LEVEL&team=money", nil, http.StatusForbidden, &refused)
	reader.MustJSON(t, http.MethodGet, "/api/v1/config/search?key=log.level", nil, http.StatusBadRequest, &badKey)

	require.Len(t, money.Items, 1)
	require.Equal(t, "billing", money.Items[0].Service)
	require.Equal(t, "err_team_not_granted", refused.Error.Code)
	require.Equal(t, "key", badKey.Error.Field)
}
