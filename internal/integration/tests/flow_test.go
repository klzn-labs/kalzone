package tests

import (
	"net/http"
	"strings"
	"testing"

	"github.com/adrianozp/platform/internal/integration"
	"github.com/stretchr/testify/require"
)

const secretValue = "s3cr3t-hunter2-value"

type people struct {
	root, mia, bob, eve *integration.Client
}

func setupPlaces(t *testing.T) (*integration.Suite, people) {
	t.Helper()
	suite := integration.NewSuite(t)
	p := people{root: suite.SignIn(t, "root"), mia: suite.SignIn(t, "mia"), bob: suite.SignIn(t, "bob"), eve: suite.SignIn(t, "eve")}
	p.root.MustJSON(t, http.MethodPost, "/api/v1/teams", map[string]string{"slug": "places", "name": "Places"}, http.StatusCreated, nil)
	p.root.MustJSON(t, http.MethodPut, "/api/v1/teams/places/members/mia", map[string]string{"role": "maintainer"}, http.StatusOK, nil)
	p.mia.MustJSON(t, http.MethodPut, "/api/v1/teams/places/members/bob", map[string]string{"role": "member"}, http.StatusOK, nil)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/services", map[string]string{
		"slug": "places-api", "name": "Places API", "owner_team": "places", "image_repository": "docker.io/adrianozdp/places-api",
	}, http.StatusCreated, nil)
	return suite, p
}

func Test_MembersEditDevButOnlyMaintainersEditProd(t *testing.T) {
	_, p := setupPlaces(t)
	change := map[string]any{"set_env": map[string]string{"LOG_LEVEL": "debug"}}

	bobDev := p.bob.Do(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", change)
	bobProd := p.bob.Do(t, http.MethodPost, "/api/v1/config/places-api/prod/revisions", change)
	miaProd := p.mia.Do(t, http.MethodPost, "/api/v1/config/places-api/prod/revisions", change)
	eveDev := p.eve.Do(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", change)

	require.Equal(t, http.StatusCreated, bobDev.Status, bobDev.Body)
	require.Equal(t, http.StatusForbidden, bobProd.Status, bobProd.Body)
	require.Equal(t, http.StatusCreated, miaProd.Status, miaProd.Body)
	require.Equal(t, http.StatusForbidden, eveDev.Status, eveDev.Body)
}

func Test_RevisionsAreNumberedAndHistoryKeepsOldValues(t *testing.T) {
	_, p := setupPlaces(t)
	path := "/api/v1/config/places-api/dev/revisions"
	p.bob.MustJSON(t, http.MethodPost, path, map[string]any{"set_env": map[string]string{"A": "1"}}, http.StatusCreated, nil)
	p.bob.MustJSON(t, http.MethodPost, path, map[string]any{"set_env": map[string]string{"A": "2"}, "message": "bump"}, http.StatusCreated, nil)
	var history struct {
		Revisions []struct {
			Number  int               `json:"number"`
			Env     map[string]string `json:"env"`
			Message string            `json:"message"`
		} `json:"items"`
	}

	p.eve.MustJSON(t, http.MethodGet, path, nil, http.StatusOK, &history)

	require.Len(t, history.Revisions, 2)
	require.Equal(t, 2, history.Revisions[0].Number)
	require.Equal(t, "bump", history.Revisions[0].Message)
	require.Equal(t, "1", history.Revisions[1].Env["A"])
}

func Test_ASecretValueNeverComesBack(t *testing.T) {
	_, p := setupPlaces(t)
	p.bob.MustJSON(t, http.MethodPut, "/api/v1/config/places-api/dev/secrets/DB_PASSWORD", map[string]string{"value": secretValue}, http.StatusOK, nil)

	reads := []string{
		"/api/v1/config/places-api/dev", "/api/v1/config/places-api/dev/revisions",
		"/api/v1/config/places-api/dev/secrets", "/api/v1/audit", "/api/v1/audit?service=places-api",
	}
	for _, path := range reads {
		resp := p.root.Do(t, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, resp.Status, path)
		require.NotContains(t, resp.Body, secretValue, path)
	}
	current := p.bob.Do(t, http.MethodGet, "/api/v1/config/places-api/dev", nil)
	require.Contains(t, current.Body, `"secrets":["DB_PASSWORD"]`)
}

func Test_TwoServicesCannotShareAHost(t *testing.T) {
	_, p := setupPlaces(t)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/services", map[string]string{"slug": "places-web", "name": "Web", "owner_team": "places"}, http.StatusCreated, nil)
	spec := func(host string) map[string]any {
		return map[string]any{"spec": map[string]any{"port": 8080, "health_path": "/healthz", "host": host, "memory_mb": 256, "cpu_millis": 250, "replicas": 1}}
	}

	first := p.bob.Do(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", spec("places.kalz.one"))
	second := p.bob.Do(t, http.MethodPost, "/api/v1/config/places-web/dev/revisions", spec("places.kalz.one"))
	foreign := p.bob.Do(t, http.MethodPost, "/api/v1/config/places-web/dev/revisions", spec("places.example.com"))

	require.Equal(t, http.StatusCreated, first.Status, first.Body)
	require.Equal(t, http.StatusConflict, second.Status, second.Body)
	require.Equal(t, http.StatusBadRequest, foreign.Status, foreign.Body)
}

func Test_TheLastMaintainerCannotLeave(t *testing.T) {
	_, p := setupPlaces(t)

	demote := p.mia.Do(t, http.MethodPut, "/api/v1/teams/places/members/mia", map[string]string{"role": "member"})
	bobPromotes := p.bob.Do(t, http.MethodPut, "/api/v1/teams/places/members/bob", map[string]string{"role": "maintainer"})

	require.Equal(t, http.StatusConflict, demote.Status, demote.Body)
	require.Contains(t, demote.Body, "err_last_maintainer")
	require.Equal(t, http.StatusForbidden, bobPromotes.Status)
}

func Test_TheAuditLogRecordsWhoDidWhat(t *testing.T) {
	_, p := setupPlaces(t)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", map[string]any{"set_env": map[string]string{"A": "1"}}, http.StatusCreated, nil)
	var audit struct {
		Events []struct {
			Actor       string `json:"actor"`
			Action      string `json:"action"`
			Environment string `json:"environment"`
		} `json:"items"`
	}

	p.eve.MustJSON(t, http.MethodGet, "/api/v1/audit?team=places", nil, http.StatusOK, &audit)

	actions := []string{}
	for _, e := range audit.Events {
		actions = append(actions, e.Actor+":"+e.Action)
	}
	require.Equal(t, "bob:config.revised", actions[0])
	require.Contains(t, strings.Join(actions, ","), "root:team.created")
	require.Contains(t, strings.Join(actions, ","), "bob:service.created")
	require.Equal(t, "dev", audit.Events[0].Environment)
}

func Test_ATeamWithServicesCannotBeDeleted(t *testing.T) {
	_, p := setupPlaces(t)

	busy := p.root.Do(t, http.MethodDelete, "/api/v1/teams/places", nil)
	p.mia.MustJSON(t, http.MethodDelete, "/api/v1/services/places-api", nil, http.StatusAccepted, nil)
	idle := p.root.Do(t, http.MethodDelete, "/api/v1/teams/places", nil)

	require.Equal(t, http.StatusConflict, busy.Status)
	require.Equal(t, http.StatusNoContent, idle.Status, idle.Body)
}
