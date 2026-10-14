package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_AReleaseIsNamedTheWayPeopleDo(t *testing.T) {
	s, p := setupPlaces(t)
	sharedBuild(t, s, p)

	for _, ref := range []string{"latest", "r1", "sha:0123456", "r1-0123456"} {
		var rel release
		p.eve.MustJSON(t, http.MethodGet, "/api/v1/releases/"+ref+"?service=places-api", nil, http.StatusOK, &rel)
		require.Equal(t, 1, rel.Number, ref)
	}
}

func Test_AReferenceThatMatchesNothingPointsAtTheList(t *testing.T) {
	s, p := setupPlaces(t)
	sharedBuild(t, s, p)
	var missing, malformed apiError

	p.eve.MustJSON(t, http.MethodGet, "/api/v1/releases/r9?service=places-api", nil, http.StatusNotFound, &missing)
	p.eve.MustJSON(t, http.MethodGet, "/api/v1/releases/sha:01?service=places-api", nil, http.StatusBadRequest, &malformed)

	require.Equal(t, "list releases with GET /api/v1/releases?service=places-api", missing.Error.Hint)
	require.Contains(t, malformed.Error.Hint, "sha:abc1234")
}

func Test_ADeployNamesItsReleaseByReference(t *testing.T) {
	s, p := setupPlaces(t)
	sharedBuild(t, s, p)
	var d deploymentView

	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-worker", "environment": "dev", "release": "latest"}, http.StatusAccepted, &d)

	require.Equal(t, 1, d.ReleaseNumber)
	require.Equal(t, "pending", d.Status)
}

func Test_ADeployRefusesAnotherServicesReleaseID(t *testing.T) {
	s, p := setupPlaces(t)
	sharedBuild(t, s, p)
	apiRelease := releasesOf(t, p.eve, "places-api")[0]
	var refused apiError

	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-worker", "environment": "dev", "release": apiRelease.ID}, http.StatusNotFound, &refused)

	require.Equal(t, "list releases with GET /api/v1/releases?service=places-worker", refused.Error.Hint)
}

func Test_AReferenceStaysWithinTheTokensTeams(t *testing.T) {
	s, p := setupPlaces(t)
	sharedBuild(t, s, p)
	otherTeamsService(t, p)
	moneyReader := agentFor(t, s, p.root, map[string]any{"name": "money-reader", "scopes": []string{"read"}, "teams": []string{"money"}})
	placesReader := agentFor(t, s, p.root, map[string]any{"name": "places-reader", "scopes": []string{"read"}, "teams": []string{"places"}})
	var hidden apiError

	moneyReader.MustJSON(t, http.MethodGet, "/api/v1/releases/latest?service=places-api", nil, http.StatusNotFound, &hidden)
	placesReader.MustJSON(t, http.MethodGet, "/api/v1/releases/latest?service=places-api", nil, http.StatusOK, nil)

	require.Equal(t, "err_not_found", hidden.Error.Code)
}
