package tests

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/klzn-labs/kalzone/internal/integration"
	"github.com/klzn-labs/kalzone/pkg/agentapi"
	"github.com/stretchr/testify/require"
)

type teardownView struct {
	Environment  string `json:"environment"`
	Status       string `json:"status"`
	DeploymentID string `json:"deployment_id"`
	RequestID    string `json:"request_id"`
	ApproveURL   string `json:"approve_url"`
}

type decommissionView struct {
	Service struct {
		Status string `json:"status"`
	} `json:"service"`
	Deleted  bool           `json:"deleted"`
	Teardown []teardownView `json:"teardown"`
}

func startDeleting(t *testing.T, c *integration.Client, slug string) decommissionView {
	t.Helper()
	var view decommissionView
	c.MustJSON(t, http.MethodDelete, "/api/v1/services/"+slug, nil, http.StatusAccepted, &view)
	return view
}

func liveOnDev(t *testing.T, s *integration.Suite, p people) {
	t.Helper()
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "dev", "release": builtRelease(t, s, p)}, http.StatusAccepted, nil)
	runToSuccess(t, s)
}

func consumedBy(t *testing.T, c *integration.Client, consumer string) {
	t.Helper()
	c.MustJSON(t, http.MethodPost, "/api/v1/queues", map[string]any{
		"name": "emails", "environment": "dev", "consumer": consumer, "path": "/hooks/emails",
	}, http.StatusCreated, nil)
}

func Test_DeletingAServiceThatNeverRanFreesItsSlugAtOnce(t *testing.T) {
	_, p := setupPlaces(t)

	view := startDeleting(t, p.mia, "places-api")
	recreated := p.bob.Do(t, http.MethodPost, "/api/v1/services", map[string]string{"slug": "places-api", "name": "Places API again", "owner_team": "places"})

	require.True(t, view.Deleted)
	require.Empty(t, view.Teardown)
	require.Equal(t, http.StatusCreated, recreated.Status, recreated.Body)
}

func Test_DeletingADeployedServiceRemovesItFromTheHostFirst(t *testing.T) {
	s, p := setupPlaces(t)
	liveOnDev(t, s, p)
	consumedBy(t, p.bob, "places-api")

	view := startDeleting(t, p.mia, "places-api")
	frozen := p.bob.Do(t, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "dev", "release": "latest"})
	again := startDeleting(t, p.mia, "places-api")
	removal, status := claim(t, s)

	require.False(t, view.Deleted)
	require.Equal(t, "decommissioning", view.Service.Status)
	require.Equal(t, []teardownView{{Environment: "dev", Status: "removing", DeploymentID: view.Teardown[0].DeploymentID}}, view.Teardown)
	require.Equal(t, http.StatusConflict, frozen.Status, frozen.Body)
	require.Equal(t, "removing", again.Teardown[0].Status)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, view.Teardown[0].DeploymentID, removal.DeploymentID)
	require.Equal(t, "remove", removal.Strategy)
	require.Equal(t, "places-api-dev", removal.Name)
	require.Empty(t, removal.Secrets)
}

func Test_TheLastSuccessfulRemovalFinishesTheDeletion(t *testing.T) {
	s, p := setupPlaces(t)
	liveOnDev(t, s, p)
	consumedBy(t, p.bob, "places-api")
	startDeleting(t, p.mia, "places-api")

	runToSuccess(t, s)
	gone := p.bob.Do(t, http.MethodGet, "/api/v1/services/places-api", nil)
	queues := p.bob.Do(t, http.MethodGet, "/api/v1/queues", nil)
	desired := s.Agent(t, http.MethodGet, agentapi.DesiredPath, nil)
	reused := p.bob.Do(t, http.MethodPost, "/api/v1/services", map[string]string{"slug": "places-api", "name": "Places API again", "owner_team": "places"})

	require.Equal(t, http.StatusNotFound, gone.Status, gone.Body)
	require.NotContains(t, queues.Body, `"emails"`)
	require.NotContains(t, desired.Body, "places-api")
	require.Equal(t, http.StatusCreated, reused.Status, reused.Body)
}

func Test_ATokenRemovingAServiceFromProdWaitsForApproval(t *testing.T) {
	s, p := setupPlaces(t)
	p.mia.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "prod", "release": builtRelease(t, s, p)}, http.StatusAccepted, nil)
	runToSuccess(t, s)
	agent := agentFor(t, s, p.mia, tokenBody("mia-cleaner", "read", "services", "services:admin", "deploy", "protected"))

	var view decommissionView
	agent.MustJSON(t, http.MethodDelete, "/api/v1/services/places-api", nil, http.StatusAccepted, &view)
	_, nothingToClaim := claim(t, s)
	approved := decide(t, p.mia, pendingChange{ID: view.Teardown[0].RequestID}, "approve", url.Values{})
	removal, claimed := claim(t, s)

	require.Equal(t, "awaiting_approval", view.Teardown[0].Status)
	require.Equal(t, "http://platform.test/requests/"+view.Teardown[0].RequestID, view.Teardown[0].ApproveURL)
	require.Equal(t, http.StatusNoContent, nothingToClaim)
	require.Equal(t, http.StatusNoContent, approved.Status, approved.Body)
	require.Equal(t, http.StatusOK, claimed)
	require.Equal(t, "places-api-prod", removal.Name)
	require.Equal(t, "remove", removal.Strategy)
}

func Test_TheServicePageStartsAndResumesADeletion(t *testing.T) {
	s, p := setupPlaces(t)
	liveOnDev(t, s, p)

	started := p.mia.Form(t, "/services/places-api/delete", url.Values{"_csrf": {p.mia.CSRFToken}, "confirm": {"places-api"}})
	page := p.mia.Do(t, http.MethodGet, "/services/places-api", nil)

	require.Equal(t, http.StatusNoContent, started.Status, started.Body)
	require.Contains(t, page.Body, "being deleted")
	require.Contains(t, page.Body, "Resume deletion")
}
