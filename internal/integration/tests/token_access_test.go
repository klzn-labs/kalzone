package tests

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/adrianozp/platform/internal/integration"
	"github.com/stretchr/testify/require"
)

type auditEvents struct {
	Events []struct {
		Action     string `json:"action"`
		Actor      string `json:"actor"`
		Token      string `json:"token"`
		TargetName string `json:"target_name"`
	} `json:"events"`
}

func deployVia(t *testing.T, c *integration.Client, releaseID, env string) integration.Response {
	t.Helper()
	return c.Do(t, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": env, "release_id": releaseID})
}

func agentFor(t *testing.T, s *integration.Suite, owner *integration.Client, body map[string]any) *integration.Client {
	t.Helper()
	return s.WithToken(t, issue(t, owner, body).Token)
}

func Test_ATokenDeploysToDevAndTheAuditNamesIt(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	bobToken := issue(t, p.bob, tokenBody("claude-local", "read", "deploy"))
	var trail auditEvents

	dev := deployVia(t, s.WithToken(t, bobToken.Token), releaseID, "dev")
	p.root.MustJSON(t, http.MethodGet, "/api/v1/audit?token="+bobToken.ID, nil, http.StatusOK, &trail)

	require.Equal(t, http.StatusAccepted, dev.Status, dev.Body)
	require.Equal(t, "deploy.requested", trail.Events[0].Action)
	require.Equal(t, "bob", trail.Events[0].Actor)
	require.Equal(t, "claude-local", trail.Events[0].Token)
}

func Test_ProdNeedsProtectedAndApprovalUnlessBypassed(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	bypass := map[string]any{"name": "root-bypass", "scopes": []string{"deploy", "protected"}, "teams": []string{"places"}, "bypass": true}
	var all auditEvents

	noScope := deployVia(t, agentFor(t, s, p.mia, tokenBody("mia-deploy", "deploy")), releaseID, "prod")
	noBypass := deployVia(t, agentFor(t, s, p.mia, tokenBody("mia-protected", "deploy", "protected")), releaseID, "prod")
	bypassed := deployVia(t, agentFor(t, s, p.root, bypass), releaseID, "prod")
	p.root.MustJSON(t, http.MethodGet, "/api/v1/audit", nil, http.StatusOK, &all)

	require.Contains(t, noScope.Body, `"required_scope":"protected"`)
	require.Equal(t, http.StatusAccepted, noBypass.Status, noBypass.Body)
	require.Contains(t, noBypass.Body, `"kind":"change_request"`)
	require.Equal(t, http.StatusAccepted, bypassed.Status, bypassed.Body)
	require.True(t, hasAction(all, "token.bypass_used"))
}

func hasAction(list auditEvents, action string) bool {
	for _, e := range list.Events {
		if e.Action == action {
			return true
		}
	}
	return false
}

func hasTarget(list auditEvents, name string) bool {
	for _, e := range list.Events {
		if e.TargetName == name {
			return true
		}
	}
	return false
}

func reviewsService() map[string]string {
	return map[string]string{"slug": "reviews-api", "name": "Reviews", "owner_team": "reviews", "image_repository": "docker.io/adrianozdp/reviews-api"}
}

func createReviews(t *testing.T, p people) {
	t.Helper()
	p.root.MustJSON(t, http.MethodPost, "/api/v1/teams", map[string]string{"slug": "reviews", "name": "Reviews"}, http.StatusCreated, nil)
}

func Test_ScopeTeamAndRoleRefusalsAreDistinct(t *testing.T) {
	s, p := setupPlaces(t)
	createReviews(t, p)
	p.root.MustJSON(t, http.MethodPut, "/api/v1/teams/reviews/members/bob", map[string]string{"role": "member"}, http.StatusOK, nil)
	change := map[string]any{"set_env": map[string]string{"A": "1"}}

	scopeResp := agentFor(t, s, p.bob, tokenBody("ro", "read")).Do(t, http.MethodPost, "/api/v1/services", reviewsService())
	teamResp := agentFor(t, s, p.bob, tokenBody("places-only", "services")).Do(t, http.MethodPost, "/api/v1/services", reviewsService())
	roleResp := agentFor(t, s, p.eve, tokenBody("eve", "services", "config")).Do(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", change)
	secretResp := agentFor(t, s, p.bob, tokenBody("config-only", "config")).Do(t, http.MethodPut, "/api/v1/config/places-api/dev/secrets/DB_PASSWORD", map[string]string{"value": "x"})

	require.Contains(t, scopeResp.Body, `"required_scope":"services"`)
	require.Equal(t, http.StatusForbidden, teamResp.Status)
	require.Contains(t, teamResp.Body, "err_team_not_granted")
	require.Contains(t, teamResp.Body, `"team":"`)
	require.Contains(t, roleResp.Body, "err_forbidden")
	require.Contains(t, secretResp.Body, `"required_scope":"secrets"`)
}

type serviceList struct {
	Services []struct {
		Slug string `json:"slug"`
	} `json:"services"`
}

func Test_ReadsOutsideTheTokensTeamsAreHidden(t *testing.T) {
	s, p := setupPlaces(t)
	createReviews(t, p)
	p.root.MustJSON(t, http.MethodPost, "/api/v1/services", reviewsService(), http.StatusCreated, nil)
	agent := agentFor(t, s, p.root, tokenBody("root-places", "read"))
	var list, other serviceList
	var events auditEvents

	agent.MustJSON(t, http.MethodGet, "/api/v1/services", nil, http.StatusOK, &list)
	agent.MustJSON(t, http.MethodGet, "/api/v1/services?team=reviews", nil, http.StatusOK, &other)
	agent.MustJSON(t, http.MethodGet, "/api/v1/audit", nil, http.StatusOK, &events)

	require.Len(t, list.Services, 1)
	require.Equal(t, "places-api", list.Services[0].Slug)
	require.Empty(t, other.Services)
	require.False(t, hasTarget(events, "reviews-api"))
}

var reviewsReads = []string{
	"/api/v1/services/reviews-api", "/api/v1/config/reviews-api/dev", "/api/v1/config/reviews-api/dev/revisions",
	"/api/v1/config/reviews-api/dev/secrets", "/api/v1/releases?service=reviews-api", "/api/v1/deployments?service=reviews-api&env=dev",
}

func Test_OtherTeamsResourcesAnswer404(t *testing.T) {
	s, p := setupPlaces(t)
	createReviews(t, p)
	p.root.MustJSON(t, http.MethodPost, "/api/v1/services", reviewsService(), http.StatusCreated, nil)
	agent := agentFor(t, s, p.root, tokenBody("root-places", "read"))

	for _, path := range reviewsReads {
		require.Equal(t, http.StatusNotFound, agent.Do(t, http.MethodGet, path, nil).Status, path)
	}
}

func Test_LosingTheRoleTakesItFromTheToken(t *testing.T) {
	s, p := setupPlaces(t)
	agent := s.WithToken(t, issue(t, p.bob, tokenBody("bob-config", "config")).Token)
	change := map[string]any{"set_env": map[string]string{"A": "1"}}
	before := agent.Do(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", change)

	p.mia.MustJSON(t, http.MethodDelete, "/api/v1/teams/places/members/bob", nil, http.StatusNoContent, nil)
	after := agent.Do(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", change)

	require.Equal(t, http.StatusCreated, before.Status, before.Body)
	require.Equal(t, http.StatusForbidden, after.Status)
	require.Contains(t, after.Body, "err_forbidden")
}

func Test_TokensCannotEscalate(t *testing.T) {
	s, p := setupPlaces(t)
	managerToken := issue(t, p.bob, tokenBody("manager", "tokens:admin", "read"))
	manager := s.WithToken(t, managerToken.Token)
	rootToken := agentFor(t, s, p.root, map[string]any{"name": "root-all", "scopes": []string{"tokens:admin", "read"}, "teams": []string{"places"}, "bypass": true})
	bypass := map[string]any{"name": "bypass", "scopes": []string{"read"}, "teams": []string{"places"}, "bypass": true}

	wider := manager.Do(t, http.MethodPost, "/api/v1/tokens", tokenBody("wider", "read", "deploy"))
	var narrower issuedToken
	manager.MustJSON(t, http.MethodPost, "/api/v1/tokens", tokenBody("narrower", "read"), http.StatusCreated, &narrower)
	memberBypass := p.bob.Do(t, http.MethodPost, "/api/v1/tokens", bypass)
	tokenBypass := rootToken.Do(t, http.MethodPost, "/api/v1/tokens", bypass)
	duplicate := p.bob.Do(t, http.MethodPost, "/api/v1/tokens", tokenBody("manager", "read"))

	require.Equal(t, http.StatusForbidden, wider.Status, wider.Body)
	require.WithinDuration(t, managerToken.ExpiresAt, narrower.ExpiresAt, time.Second)
	require.Equal(t, http.StatusForbidden, memberBypass.Status, memberBypass.Body)
	require.Equal(t, http.StatusForbidden, tokenBypass.Status, tokenBypass.Body)
	require.Equal(t, http.StatusConflict, duplicate.Status, duplicate.Body)
}

func Test_WhoamiAndTheSecretIsShownOnce(t *testing.T) {
	s, p := setupPlaces(t)
	issued := issue(t, p.bob, tokenBody("claude-local", "read", "deploy"))
	agent := s.WithToken(t, issued.Token)
	var who whoami

	agent.MustJSON(t, http.MethodGet, "/api/v1/whoami", nil, http.StatusOK, &who)
	list := p.bob.Do(t, http.MethodGet, "/api/v1/tokens", nil)
	fromToken := agent.Do(t, http.MethodGet, "/api/v1/tokens", nil)

	require.Equal(t, "bob", who.Owner.Login)
	require.Equal(t, []string{"read", "deploy"}, who.Token.Scopes)
	require.Equal(t, []string{"places"}, who.Token.Teams)
	require.Equal(t, issued.Prefix, who.Token.Prefix)
	require.Equal(t, http.StatusOK, list.Status)
	require.Contains(t, list.Body, issued.Prefix)
	require.False(t, strings.Contains(list.Body, issued.Token))
	require.Contains(t, fromToken.Body, `"required_scope":"tokens:admin"`)
}

func Test_ManagingTokensFromATokenNeedsTokensAdmin(t *testing.T) {
	s, p := setupPlaces(t)
	issued := issue(t, p.bob, tokenBody("reader", "read"))
	agent := s.WithToken(t, issued.Token)

	create := agent.Do(t, http.MethodPost, "/api/v1/tokens", tokenBody("child", "read"))
	revoke := agent.Do(t, http.MethodDelete, "/api/v1/tokens/"+issued.ID, nil)
	list := p.bob.Do(t, http.MethodGet, "/api/v1/tokens", nil)

	require.Contains(t, create.Body, `"required_scope":"tokens:admin"`)
	require.Contains(t, revoke.Body, `"required_scope":"tokens:admin"`)
	require.NotContains(t, strings.ToLower(list.Body), "hash")
}

func Test_QueuesOutsideTheTokensTeamsAreHidden(t *testing.T) {
	s, p := setupQueue(t)
	createReviews(t, p)
	p.root.MustJSON(t, http.MethodPost, "/api/v1/services", reviewsService(), http.StatusCreated, nil)
	p.root.MustJSON(t, http.MethodPost, "/api/v1/queues", map[string]any{"name": "review-sync", "environment": "dev", "consumer": "reviews-api", "path": "/hooks/sync"}, http.StatusCreated, nil)
	createQueue(t, p.bob, "dev", http.StatusCreated)
	agent := agentFor(t, s, p.root, tokenBody("root-places", "read"))
	var own, other queueList

	agent.MustJSON(t, http.MethodGet, "/api/v1/queues", nil, http.StatusOK, &own)
	agent.MustJSON(t, http.MethodGet, "/api/v1/queues?team=reviews", nil, http.StatusOK, &other)
	hidden := agent.Do(t, http.MethodGet, "/api/v1/queues/dev/review-sync", nil)

	require.Len(t, own.Queues, 1)
	require.Equal(t, "place-import", own.Queues[0].Name)
	require.Empty(t, other.Queues)
	require.Equal(t, http.StatusNotFound, hidden.Status, hidden.Body)
}

type queueList struct {
	Queues []queueDTO `json:"queues"`
}
