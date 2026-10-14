package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/adrianozp/platform/internal/integration"
	"github.com/adrianozp/platform/internal/mcpserver"
	"github.com/adrianozp/platform/pkg/agentapi"
	"github.com/adrianozp/platform/pkg/scope"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

type bearerTransport struct {
	token string
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func mcpSession(t *testing.T, s *integration.Suite, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint: s.Server.URL + mcpserver.Path, HTTPClient: &http.Client{Transport: bearerTransport{token}}, DisableStandaloneSSE: true,
	}
	session, err := client.Connect(context.Background(), transport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func listTools(t *testing.T, session *mcp.ClientSession) map[string]*mcp.Tool {
	t.Helper()
	listed, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	tools := map[string]*mcp.Tool{}
	for _, tool := range listed.Tools {
		tools[tool.Name] = tool
	}
	return tools
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	return result.Content[0].(*mcp.TextContent).Text, result.IsError
}

func allScopes() []string {
	names := []string{}
	for _, s := range scope.All {
		names = append(names, string(s))
	}
	return names
}

func Test_EveryOperationIsAToolOrExcluded(t *testing.T) {
	s, p := setupPlaces(t)
	tools := listTools(t, mcpSession(t, s, issue(t, p.root, tokenBody("everything", allScopes()...)).Token))

	for _, op := range s.Catalog.Operations() {
		_, listed := tools[op.ID]
		require.True(t, listed != mcpserver.Excluded(op.ID), op.ID)
	}
	require.Contains(t, tools, "wait")
	for name, tool := range tools {
		raw, err := json.Marshal(tool.InputSchema)
		require.NoError(t, err, name)
		var schema jsonschema.Schema
		require.NoError(t, json.Unmarshal(raw, &schema), name)
		_, err = schema.Resolve(nil)
		require.NoError(t, err, name)
	}
}

func Test_ATokenSeesOnlyTheToolsOfItsScopes(t *testing.T) {
	s, p := setupPlaces(t)

	reader := listTools(t, mcpSession(t, s, issue(t, p.bob, tokenBody("reader", "read")).Token))
	deployer := listTools(t, mcpSession(t, s, issue(t, p.bob, tokenBody("deployer", "read", "deploy")).Token))

	require.Contains(t, reader, "listDeployments")
	require.Contains(t, reader, "getWhoami")
	require.NotContains(t, reader, "createDeployment")
	require.Contains(t, deployer, "createDeployment")
}

func finishTheNextDeployment(s *integration.Suite) <-chan int {
	finished := make(chan int, 1)
	go func() { finished <- finishOnceClaimed(s) }()
	return finished
}

func finishOnceClaimed(s *integration.Suite) int {
	for range 100 {
		if status, claimed := finishIfClaimed(s); claimed {
			return status
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0
}

func finishIfClaimed(s *integration.Suite) (int, bool) {
	status, body := agentPost(s, agentapi.ClaimPath, nil)
	if status != http.StatusOK {
		return 0, false
	}
	var w agentapi.Workload
	_ = json.Unmarshal(body, &w)
	done, _ := agentPost(s, agentapi.FinishPath(w.DeploymentID), agentapi.Finish{Succeeded: true, Message: "live"})
	return done, true
}

func Test_AnAgentRegistersAndDeploysThroughTools(t *testing.T) {
	s, p := setupPlaces(t)
	s.Registry.Has("adrianozdp/places-api", "1.0.0", digest)
	session := mcpSession(t, s, issue(t, p.bob, tokenBody("claude-local", "read", "builds", "deploy")).Token)
	var rel release
	var d deploymentView

	registered, failed := callTool(t, session, "registerRelease", map[string]any{"service": "places-api", "tag": "1.0.0"})
	require.False(t, failed, registered)
	require.NoError(t, json.Unmarshal([]byte(registered), &rel))
	finished := finishTheNextDeployment(s)
	deployed, failed := callTool(t, session, "createDeployment", map[string]any{"service": "places-api", "environment": "dev", "release": rel.ID, "wait": true})

	require.False(t, failed, deployed)
	require.Equal(t, http.StatusNoContent, <-finished)
	require.NoError(t, json.Unmarshal([]byte(deployed), &d))
	require.Equal(t, "succeeded", d.Status)
}

func Test_AProdDeployThroughAToolBecomesAChangeRequest(t *testing.T) {
	s, p := setupPlaces(t)
	releaseID := builtRelease(t, s, p)
	session := mcpSession(t, s, issue(t, p.mia, tokenBody("mia-agent", "read", "deploy", "protected")).Token)

	answer, failed := callTool(t, session, "createDeployment", map[string]any{"service": "places-api", "environment": "prod", "release": releaseID})

	require.False(t, failed, answer)
	require.Contains(t, answer, `"kind":"change_request"`)
	require.Contains(t, answer, `"approve_url"`)
}

func Test_AnAPIErrorComesBackAsAToolError(t *testing.T) {
	s, p := setupPlaces(t)
	session := mcpSession(t, s, issue(t, p.bob, tokenBody("reader", "read")).Token)

	answer, failed := callTool(t, session, "getDeployment", map[string]any{"id": "nope"})

	require.True(t, failed)
	require.Contains(t, answer, `"code":"err_not_found"`)
	require.Contains(t, answer, `"hint"`)
}

func Test_APurgeNeedsTheQueueNameAgain(t *testing.T) {
	s, p := setupPlaces(t)
	session := mcpSession(t, s, issue(t, p.bob, tokenBody("services", "read", "services")).Token)

	answer, failed := callTool(t, session, "purgeDeadLetters", map[string]any{"env": "dev", "name": "jobs", "confirm": "job"})

	require.True(t, failed)
	require.Contains(t, answer, `"field":"confirm"`)
}

func Test_MCPNeedsABearerToken(t *testing.T) {
	s, p := setupPlaces(t)
	headers := map[string]string{"Content-Type": "application/json", "Accept": "application/json, text/event-stream"}
	empty := map[string]string{"Content-Type": "application/json", "Accept": "application/json, text/event-stream", "Authorization": "Bearer "}

	none := s.Raw(t, http.MethodPost, mcpserver.Path, []byte(initialize), headers)
	blank := s.Raw(t, http.MethodPost, mcpserver.Path, []byte(initialize), empty)
	session := p.bob.Send(t, http.MethodPost, mcpserver.Path, "application/json", json.RawMessage(initialize))

	for _, resp := range []integration.Response{none, blank, session} {
		require.Equal(t, http.StatusUnauthorized, resp.Status, resp.Body)
		require.Equal(t, "Bearer", resp.Header.Get("WWW-Authenticate"))
	}
}

func Test_TheAuditNamesTheTokenBehindAToolCall(t *testing.T) {
	s, p := setupPlaces(t)
	issued := issue(t, p.bob, tokenBody("claude-local", "read", "config"))
	session := mcpSession(t, s, issued.Token)
	var trail auditEvents

	answer, failed := callTool(t, session, "reviseConfig", map[string]any{"service": "places-api", "env": "dev", "set_env": map[string]string{"A": "1"}})
	p.root.MustJSON(t, http.MethodGet, "/api/v1/audit?token="+issued.ID, nil, http.StatusOK, &trail)

	require.False(t, failed, answer)
	require.Equal(t, "claude-local", trail.Events[0].Token)
}

func Test_TheDocumentDescribesTheMergePatchBody(t *testing.T) {
	_, p := setupPlaces(t)
	var doc struct {
		Paths map[string]map[string]struct {
			RequestBody struct {
				Content map[string]struct {
					Schema map[string]any `json:"schema"`
				} `json:"content"`
			} `json:"requestBody"`
		} `json:"paths"`
	}

	p.bob.MustJSON(t, http.MethodGet, "/api/v1/openapi.json", nil, http.StatusOK, &doc)

	body := doc.Paths["/api/v1/config/{service}/{env}"]["patch"].RequestBody.Content["application/merge-patch+json"]
	require.Contains(t, body.Schema, "$ref")
}
