package tests

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/internal/integration"
	"github.com/adrianozp/platform/pkg/agentapi"
	"github.com/stretchr/testify/require"
)

var outsideTheDocument = map[string]bool{
	"POST /api/v1/releases/:id/started": true,
	"POST /api/v1/releases/:id/built":   true,
	"POST /api/v1/releases/:id/failed":  true,
}

type apiError struct {
	Error struct {
		Code    string         `json:"code"`
		Field   string         `json:"field"`
		Hint    string         `json:"hint"`
		Details map[string]any `json:"details"`
	} `json:"error"`
}

type page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
}

func Test_EveryAPIRouteIsDeclared(t *testing.T) {
	s := setupSuiteOnly(t)
	declared, served := map[string]bool{}, map[string]bool{}

	for _, op := range s.Catalog.Operations() {
		declared[op.Method+" "+apispec.BasePath+op.Path] = true
	}
	for _, route := range s.Routes() {
		key := route.Method + " " + route.Path
		if integration.IsAPI(route.Path) && !outsideTheDocument[key] {
			served[key] = true
		}
	}

	require.Equal(t, declared, served)
}

func Test_AnyTokenReadsTheDocument(t *testing.T) {
	s, p := setupPlaces(t)
	agent := agentFor(t, s, p.bob, tokenBody("deployer", "deploy"))
	var doc struct {
		OpenAPI string                               `json:"openapi"`
		Paths   map[string]map[string]map[string]any `json:"paths"`
	}

	agent.MustJSON(t, http.MethodGet, "/api/v1/openapi.json", nil, http.StatusOK, &doc)

	deploy := doc.Paths["/api/v1/deployments"]["post"]
	require.Equal(t, "3.1.0", doc.OpenAPI)
	require.Equal(t, "createDeployment", deploy["operationId"])
	require.Equal(t, []any{map[string]any{"token": []any{"deploy"}}, map[string]any{"session": []any{}}}, deploy["security"])
	require.Contains(t, doc.Paths, "/api/v1/deployments/{id}/events")
}

func Test_AnUnknownFieldIsRefusedWithTheAcceptedFields(t *testing.T) {
	_, p := setupPlaces(t)
	var refused apiError

	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "dev", "release_id": "r1"}, http.StatusBadRequest, &refused)

	require.Equal(t, "err_invalid_input", refused.Error.Code)
	require.Equal(t, "release_id", refused.Error.Field)
	require.Contains(t, refused.Error.Hint, "this body accepts: service, environment, targets, release, strategy")
	require.Equal(t, []any{"service", "environment", "targets", "release", "strategy"}, refused.Error.Details["accepted"])
}

func Test_ListsPageWithACursor(t *testing.T) {
	s, p := setupPlaces(t)
	for _, tag := range []string{"1.0.0", "1.0.1", "1.0.2"} {
		s.Registry.Has("adrianozdp/places-api", tag, digest)
		p.bob.MustJSON(t, http.MethodPost, "/api/v1/releases/registrations", map[string]string{"service": "places-api", "tag": tag}, http.StatusCreated, nil)
	}
	var first, second page[release]
	var badLimit apiError

	p.eve.MustJSON(t, http.MethodGet, "/api/v1/releases?service=places-api&limit=2", nil, http.StatusOK, &first)
	p.eve.MustJSON(t, http.MethodGet, "/api/v1/releases?service=places-api&limit=2&cursor="+first.NextCursor, nil, http.StatusOK, &second)
	p.eve.MustJSON(t, http.MethodGet, "/api/v1/releases?service=places-api&limit=500", nil, http.StatusBadRequest, &badLimit)

	require.Equal(t, []int{3, 2}, []int{first.Items[0].Number, first.Items[1].Number})
	require.Len(t, second.Items, 1)
	require.Equal(t, 1, second.Items[0].Number)
	require.Empty(t, second.NextCursor)
	require.Equal(t, "limit", badLimit.Error.Field)
}

func Test_TheAuditLogPagesWithACursor(t *testing.T) {
	_, p := setupPlaces(t)
	var first, second page[map[string]any]

	p.root.MustJSON(t, http.MethodGet, "/api/v1/audit?limit=1", nil, http.StatusOK, &first)
	p.root.MustJSON(t, http.MethodGet, "/api/v1/audit?limit=1&cursor="+first.NextCursor, nil, http.StatusOK, &second)

	require.Equal(t, "service.created", first.Items[0]["action"])
	require.NotEqual(t, first.Items[0]["id"], second.Items[0]["id"])
	require.NotEmpty(t, second.NextCursor)
}

type logLine struct {
	Message string `json:"message"`
}

type deploymentDetail struct {
	Summary struct {
		Total         int       `json:"total"`
		ProgressLines int       `json:"progress_lines"`
		Tail          []logLine `json:"tail"`
		Log           string    `json:"log"`
	} `json:"events_summary"`
}

func Test_ADeploymentSummarisesItsLog(t *testing.T) {
	s, p := setupPlaces(t)
	d := deployDev(t, s, p)
	w, _ := claim(t, s)
	report(t, s, w.DeploymentID, 30)
	var detail deploymentDetail

	p.eve.MustJSON(t, http.MethodGet, "/api/v1/deployments/"+d.ID, nil, http.StatusOK, &detail)

	require.Equal(t, 60, detail.Summary.Total)
	require.Equal(t, 30, detail.Summary.ProgressLines)
	require.Len(t, detail.Summary.Tail, 20)
	require.Equal(t, "step 30", detail.Summary.Tail[19].Message)
	require.Equal(t, "/api/v1/deployments/"+d.ID+"/events", detail.Summary.Log)
}

func Test_ARunningDeploymentsLogCanBeFollowed(t *testing.T) {
	s, p := setupPlaces(t)
	d := deployDev(t, s, p)
	w, _ := claim(t, s)
	report(t, s, w.DeploymentID, 2)
	var tail, followed page[logLine]

	p.eve.MustJSON(t, http.MethodGet, "/api/v1/deployments/"+d.ID+"/events?tail=2", nil, http.StatusOK, &tail)
	finish := s.Agent(t, http.MethodPost, agentapi.FinishPath(w.DeploymentID), agentapi.Finish{Succeeded: true, Message: "live"})
	p.eve.MustJSON(t, http.MethodGet, "/api/v1/deployments/"+d.ID+"/events?cursor="+tail.NextCursor, nil, http.StatusOK, &followed)

	require.Equal(t, []logLine{{"5f70bf18a002 Downloading [==>  ]"}, {"step 2"}}, tail.Items)
	require.NotEmpty(t, tail.NextCursor)
	require.Equal(t, http.StatusNoContent, finish.Status)
	require.Empty(t, followed.Items)
	require.Empty(t, followed.NextCursor)
}

func deployDev(t *testing.T, s *integration.Suite, p people) deploymentView {
	t.Helper()
	var d deploymentView
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/deployments", map[string]string{"service": "places-api", "environment": "dev", "release": builtRelease(t, s, p)}, http.StatusAccepted, &d)
	return d
}

func report(t *testing.T, s *integration.Suite, deploymentID string, steps int) {
	t.Helper()
	lines := []string{}
	for i := 1; i <= steps; i++ {
		lines = append(lines, fmt.Sprintf("5f70bf18a0%02d Downloading [==>  ]", i), fmt.Sprintf("step %d", i))
	}
	require.Equal(t, http.StatusNoContent, s.Agent(t, http.MethodPost, agentapi.EventsPath(deploymentID), agentapi.Events{Lines: lines}).Status)
}

func Test_TokensNameTheirTeamsBySlug(t *testing.T) {
	_, p := setupPlaces(t)
	issue(t, p.bob, tokenBody("claude-local", "read"))
	var tokens page[struct {
		Teams []string `json:"teams"`
	}]

	p.bob.MustJSON(t, http.MethodGet, "/api/v1/tokens", nil, http.StatusOK, &tokens)

	require.Equal(t, []string{"places"}, tokens.Items[0].Teams)
}

func Test_AnUnknownRequestStatusIsRefused(t *testing.T) {
	_, p := setupPlaces(t)
	var refused apiError

	p.mia.MustJSON(t, http.MethodGet, "/api/v1/requests?status=bogus", nil, http.StatusBadRequest, &refused)

	require.Equal(t, "status", refused.Error.Field)
	require.Contains(t, refused.Error.Hint, "pending")
}

func Test_AListWithoutItsRequiredQueryNamesTheParameter(t *testing.T) {
	_, p := setupPlaces(t)
	var releases, deployments, deploymentsWithoutEnv apiError

	p.eve.MustJSON(t, http.MethodGet, "/api/v1/releases", nil, http.StatusBadRequest, &releases)
	p.eve.MustJSON(t, http.MethodGet, "/api/v1/deployments?env=dev", nil, http.StatusBadRequest, &deployments)
	p.eve.MustJSON(t, http.MethodGet, "/api/v1/deployments?service=places-api", nil, http.StatusBadRequest, &deploymentsWithoutEnv)

	requireRefusedField(t, releases, "service", "send ?service=<slug>")
	requireRefusedField(t, deployments, "service", "send ?service=<slug>")
	requireRefusedField(t, deploymentsWithoutEnv, "env", "send ?env=<environment>")
}

func requireRefusedField(t *testing.T, refused apiError, field, hint string) {
	t.Helper()
	require.Equal(t, "err_invalid_input", refused.Error.Code)
	require.Equal(t, field, refused.Error.Field)
	require.Contains(t, refused.Error.Hint, hint)
}

func Test_TokenRefusalsHintAtTheFix(t *testing.T) {
	s, p := setupQueue(t)
	createReviews(t, p)
	p.root.MustJSON(t, http.MethodPut, "/api/v1/teams/reviews/members/bob", map[string]string{"role": "member"}, http.StatusOK, nil)
	createQueue(t, p.mia, "prod", http.StatusCreated)
	var scopeRefusal, teamRefusal, approvalRefusal apiError

	agentFor(t, s, p.bob, tokenBody("ro", "read")).MustJSON(t, http.MethodPost, "/api/v1/services", reviewsService(), http.StatusForbidden, &scopeRefusal)
	agentFor(t, s, p.bob, tokenBody("places-only", "services")).MustJSON(t, http.MethodPost, "/api/v1/services", reviewsService(), http.StatusForbidden, &teamRefusal)
	agentFor(t, s, p.mia, tokenBody("mia-protected", "services", "protected")).MustJSON(t, http.MethodPost, "/api/v1/queues/prod/place-import/pause", nil, http.StatusForbidden, &approvalRefusal)

	require.Equal(t, "err_missing_scope", scopeRefusal.Error.Code)
	require.Contains(t, scopeRefusal.Error.Hint, "a token with the services scope")
	require.Equal(t, "err_team_not_granted", teamRefusal.Error.Code)
	require.Contains(t, teamRefusal.Error.Hint, "ask for a token granted this team")
	require.Equal(t, "err_approval_required", approvalRefusal.Error.Code)
	require.Contains(t, approvalRefusal.Error.Hint, "use a bypass token")
}

var listEndpoints = []string{
	"/api/v1/environments",
	"/api/v1/users",
	"/api/v1/services",
	"/api/v1/teams",
	"/api/v1/teams/places/members",
	"/api/v1/releases?service=places-api",
	"/api/v1/deployments?service=places-api&env=dev",
	"/api/v1/config/places-api/dev/revisions",
	"/api/v1/queues",
	"/api/v1/queues/dev/place-import/versions",
	"/api/v1/queues/dev/place-import/dead",
	"/api/v1/requests",
	"/api/v1/tokens",
	"/api/v1/audit",
}

func Test_EveryListAnswersWithinTheDocument(t *testing.T) {
	s, p := setupQueue(t)
	createQueue(t, p.bob, "dev", http.StatusCreated)
	deployDev(t, s, p)
	p.bob.MustJSON(t, http.MethodPost, "/api/v1/config/places-api/dev/revisions", map[string]any{"set_env": map[string]string{"A": "1"}}, http.StatusCreated, nil)
	issue(t, p.bob, tokenBody("claude-local", "read"))

	for _, endpoint := range listEndpoints {
		t.Run(endpoint, func(t *testing.T) {
			var listed page[map[string]any]
			p.root.MustJSON(t, http.MethodGet, endpoint, nil, http.StatusOK, &listed)
			require.NotNil(t, listed.Items)
		})
	}
}
