package tests

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/adrianozp/platform/internal/integration"
	"github.com/stretchr/testify/require"
)

type pendingChange struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	Status     string `json:"status"`
	ApproveURL string `json:"approve_url"`
	Poll       string `json:"poll"`
}

type requestView struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Reason string `json:"reason"`
	Result string `json:"result"`
	Error  *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func prodAgent(t *testing.T, s *integration.Suite, p people) *integration.Client {
	t.Helper()
	return agentFor(t, s, p.mia, tokenBody("mia-agent", "read", "services", "config", "secrets", "deploy", "protected"))
}

func ask(t *testing.T, agent *integration.Client, method, path string, body any) pendingChange {
	t.Helper()
	var pending pendingChange
	agent.MustJSON(t, method, path, body, http.StatusAccepted, &pending)
	return pending
}

func poll(t *testing.T, c *integration.Client, pending pendingChange) requestView {
	t.Helper()
	var r requestView
	c.MustJSON(t, http.MethodGet, pending.Poll, nil, http.StatusOK, &r)
	return r
}

func decide(t *testing.T, c *integration.Client, pending pendingChange, verb string, values url.Values) integration.Response {
	t.Helper()
	values.Set("_csrf", c.CSRFToken)
	return c.Form(t, "/requests/"+pending.ID+"/"+verb, values)
}

func Test_AnAgentsProdDeployWaitsForAHuman(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	agent := prodAgent(t, s, p)

	pending := ask(t, agent, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "prod", "release_id": releaseID})
	before := poll(t, agent, pending)
	approved := decide(t, p.mia, pending, "approve", url.Values{})
	after := poll(t, agent, pending)

	require.Equal(t, "change_request", pending.Kind)
	require.Equal(t, "http://platform.test/requests/"+pending.ID, pending.ApproveURL)
	require.Equal(t, "pending", before.Status)
	require.Equal(t, http.StatusNoContent, approved.Status, approved.Body)
	require.Equal(t, "executed", after.Status)
	require.Contains(t, after.Result, "deployment #1")
	require.Equal(t, map[string]any{"request_id": pending.ID, "approved_by": "mia"}, approvalOf(t, p.root, "deploy.requested"))
}

type detailedEvents struct {
	Events []struct {
		Action  string         `json:"action"`
		Details map[string]any `json:"details"`
	} `json:"events"`
}

func approvalOf(t *testing.T, c *integration.Client, action string) map[string]any {
	t.Helper()
	var trail detailedEvents
	c.MustJSON(t, http.MethodGet, "/api/v1/audit", nil, http.StatusOK, &trail)
	for _, e := range trail.Events {
		if e.Action == action {
			return map[string]any{"request_id": e.Details["request_id"], "approved_by": e.Details["approved_by"]}
		}
	}
	return nil
}

func Test_OnlyAMaintainerOrAdminApprovesAndOnlyInTheBrowser(t *testing.T) {
	s, p := setupPlaces(t)
	agent := prodAgent(t, s, p)
	pending := askToSet(t, agent, "A", "1")

	byMember := decide(t, p.bob, pending, "approve", url.Values{})
	byToken := agent.Do(t, http.MethodPost, "/requests/"+pending.ID+"/approve", nil)
	byAdmin := decide(t, p.root, pending, "approve", url.Values{})

	require.Equal(t, http.StatusUnprocessableEntity, byMember.Status, byMember.Body)
	require.Equal(t, http.StatusUnauthorized, byToken.Status, byToken.Body)
	require.Equal(t, http.StatusNoContent, byAdmin.Status, byAdmin.Body)
	require.Equal(t, "executed", poll(t, agent, pending).Status)
}

func Test_ASecretRequestNeverShowsItsValue(t *testing.T) {
	s, p := setupPlaces(t)
	agent := prodAgent(t, s, p)

	pending := ask(t, agent, http.MethodPut, "/api/v1/config/places-api/prod/secrets/DB_PASSWORD", map[string]string{"value": "hunter2-prod"})
	page := p.mia.Do(t, http.MethodGet, "/requests/"+pending.ID, nil)
	api := agent.Do(t, http.MethodGet, pending.Poll, nil)
	decide(t, p.mia, pending, "approve", url.Values{})
	secrets := p.mia.Do(t, http.MethodGet, "/api/v1/config/places-api/prod/secrets", nil)

	require.Equal(t, http.StatusOK, page.Status, page.Body)
	require.Contains(t, page.Body, "secret.DB_PASSWORD")
	require.Contains(t, page.Body, "new value")
	require.NotContains(t, page.Body, "hunter2-prod")
	require.Equal(t, http.StatusOK, api.Status, api.Body)
	require.Contains(t, api.Body, `"id":"`+pending.ID+`"`)
	require.NotContains(t, api.Body, "hunter2-prod")
	require.Equal(t, http.StatusOK, secrets.Status, secrets.Body)
	require.Contains(t, secrets.Body, "DB_PASSWORD")
}

func setProd(t *testing.T, c *integration.Client, key, value string) {
	t.Helper()
	c.MustJSON(t, http.MethodPost, "/api/v1/config/places-api/prod/revisions", map[string]any{"set_env": map[string]string{key: value}}, http.StatusCreated, nil)
}

func askToSet(t *testing.T, agent *integration.Client, key, value string) pendingChange {
	t.Helper()
	return ask(t, agent, http.MethodPost, "/api/v1/config/places-api/prod/revisions", map[string]any{"set_env": map[string]string{key: value}})
}

func prodEnv(t *testing.T, c *integration.Client) map[string]string {
	t.Helper()
	var current struct {
		Env map[string]string `json:"env"`
	}
	c.MustJSON(t, http.MethodGet, "/api/v1/config/places-api/prod", nil, http.StatusOK, &current)
	return current.Env
}

func Test_ApprovalReappliesOnTheCurrentConfigAndFailsOnARealConflict(t *testing.T) {
	s, p := setupPlaces(t)
	agent := prodAgent(t, s, p)

	clean := askToSet(t, agent, "A", "1")
	setProd(t, p.mia, "B", "2")
	decide(t, p.mia, clean, "approve", url.Values{})
	clash := askToSet(t, agent, "A", "3")
	setProd(t, p.mia, "A", "4")
	warning := p.mia.Do(t, http.MethodGet, "/requests/"+clash.ID, nil)
	decide(t, p.mia, clash, "approve", url.Values{})

	failed := poll(t, agent, clash)
	require.Equal(t, "executed", poll(t, agent, clean).Status)
	require.Contains(t, warning.Body, "approving will fail")
	require.Equal(t, "failed", failed.Status)
	require.Equal(t, "err_stale", failed.Error.Code)
	require.Equal(t, map[string]string{"A": "4", "B": "2"}, prodEnv(t, p.mia))
}

func Test_ANewerRequestSupersedesTheOlderOne(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	agent := prodAgent(t, s, p)
	deploy := map[string]string{"service": "places-api", "environment": "prod", "release_id": releaseID}

	first := ask(t, agent, http.MethodPost, "/api/v1/deployments", deploy)
	second := ask(t, agent, http.MethodPost, "/api/v1/deployments", deploy)

	older := poll(t, agent, first)
	require.Equal(t, "cancelled", older.Status)
	require.Equal(t, "superseded by "+second.ID, older.Reason)
	require.Equal(t, "pending", poll(t, agent, second).Status)
}

func Test_RejectionsAndCancellationsReachTheAgent(t *testing.T) {
	s, p := setupPlaces(t)
	agent := prodAgent(t, s, p)
	stranger := agentFor(t, s, p.bob, tokenBody("bob-reader", "read"))

	rejected := askToSet(t, agent, "A", "1")
	decide(t, p.mia, rejected, "reject", url.Values{"reason": {"not during the sale"}})
	cancelled := ask(t, agent, http.MethodPut, "/api/v1/config/places-api/prod/secrets/TOKEN", map[string]string{"value": "x"})
	notTheirs := stranger.Do(t, http.MethodPost, "/api/v1/requests/"+cancelled.ID+"/cancel", nil)
	agent.MustJSON(t, http.MethodPost, "/api/v1/requests/"+cancelled.ID+"/cancel", nil, http.StatusOK, nil)

	require.Equal(t, "rejected", poll(t, agent, rejected).Status)
	require.Equal(t, "not during the sale", poll(t, agent, rejected).Reason)
	require.Equal(t, http.StatusForbidden, notTheirs.Status, notTheirs.Body)
	require.Equal(t, "cancelled", poll(t, agent, cancelled).Status)
}

func Test_TheNavCountsWhatTheViewerMayApprove(t *testing.T) {
	s, p := setupPlaces(t)
	agent := prodAgent(t, s, p)
	askToSet(t, agent, "A", "1")

	mia := p.mia.Do(t, http.MethodGet, "/requests/badge", nil)
	bob := p.bob.Do(t, http.MethodGet, "/requests/badge", nil)
	list := p.bob.Do(t, http.MethodGet, "/requests", nil)

	require.Contains(t, mia.Body, ">1<")
	require.Empty(t, bob.Body)
	require.Contains(t, list.Body, "change env.A")
}

func Test_QueueEditsOnProdWaitToo(t *testing.T) {
	s, p := setupQueue(t)
	agent := prodAgent(t, s, p)

	pending := ask(t, agent, http.MethodPost, "/api/v1/queues", map[string]any{
		"name": "place-import", "environment": "prod", "consumer": "places-worker", "path": "/hooks/place-import",
	})
	decide(t, p.mia, pending, "approve", url.Values{})
	var q queueDTO
	p.mia.MustJSON(t, http.MethodGet, "/api/v1/queues/prod/place-import", nil, http.StatusOK, &q)

	require.Equal(t, "queue version 1", poll(t, agent, pending).Result)
	require.Equal(t, "places-worker", q.Consumer)
}
