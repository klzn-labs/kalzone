package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

type echoed struct {
	Method      string `json:"method"`
	URI         string `json:"uri"`
	Body        string `json:"body"`
	ContentType string `json:"content_type"`
	Auth        string `json:"auth"`
	Forwarded   string `json:"forwarded"`
	Remote      string `json:"remote"`
}

type echoEngine struct {
	*gin.Engine
	purges int
}

func echo(c *gin.Context) {
	body, _ := io.ReadAll(c.Request.Body)
	c.JSON(http.StatusOK, echoed{
		Method: c.Request.Method, URI: c.Request.URL.RequestURI(), Body: string(body), ContentType: c.GetHeader("Content-Type"),
		Auth: c.GetHeader("Authorization"), Forwarded: c.GetHeader("X-Forwarded-For"), Remote: c.Request.RemoteAddr,
	})
}

func newEchoEngine() *echoEngine {
	gin.SetMode(gin.TestMode)
	e := &echoEngine{Engine: gin.New()}
	e.POST("/api/v1/queues/:env/:name", echo)
	e.PUT("/api/v1/raw", echo)
	e.GET("/api/v1/deployments/:id", func(c *gin.Context) {
		c.Header("Retry-After", "5")
		c.JSON(http.StatusAccepted, gin.H{"id": c.Param("id"), "uri": c.Request.URL.RequestURI()})
	})
	e.GET("/api/v1/missing", func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": "no such thing", "hint": "list things first"}})
	})
	e.DELETE("/api/v1/queues/:env/:name/dead", func(c *gin.Context) {
		e.purges++
		c.Status(http.StatusNoContent)
	})
	return e
}

func origin() *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	r.Header.Set("Authorization", "Bearer plat_x")
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	r.RemoteAddr = "10.1.2.3:4444"
	return r
}

var queueCall = call{method: http.MethodPost, path: "/queues/:env/:name", input: input{
	Path: []string{"env", "name"}, Query: []string{"wait", "limit"}, Fields: []string{"color", "size"},
	Body: bodyFields, ContentType: "application/merge-patch+json",
}}

var purgeCall = call{method: http.MethodDelete, path: "/queues/:env/:name/dead", input: input{Path: []string{"env", "name"}}, confirm: confirmedName}

func invoke(t *testing.T, e http.Handler, c call, raw string) *mcp.CallToolResult {
	t.Helper()
	args, err := arguments(json.RawMessage(raw))
	require.NoError(t, err)
	return c.run(context.Background(), e, origin(), args)
}

func textOf(r *mcp.CallToolResult, i int) string {
	return r.Content[i].(*mcp.TextContent).Text
}

func echoOf(t *testing.T, r *mcp.CallToolResult) echoed {
	t.Helper()
	require.False(t, r.IsError, textOf(r, 0))
	var e echoed
	require.NoError(t, json.Unmarshal([]byte(textOf(r, 0)), &e))
	return e
}

func Test_Run_BuildsTheAPIRequestFromTheArguments(t *testing.T) {
	got := echoOf(t, invoke(t, newEchoEngine(), queueCall, `{"env":"dev","name":"a b","limit":20,"wait":true,"color":"red"}`))

	require.Equal(t, http.MethodPost, got.Method)
	require.Equal(t, "/api/v1/queues/dev/a%20b?limit=20&wait=120s", got.URI)
	require.JSONEq(t, `{"color":"red"}`, got.Body)
	require.Equal(t, "application/merge-patch+json", got.ContentType)
}

func Test_Run_CopiesTheCallersIdentityAndAddress(t *testing.T) {
	got := echoOf(t, invoke(t, newEchoEngine(), queueCall, `{"env":"dev","name":"q"}`))

	require.Equal(t, "Bearer plat_x", got.Auth)
	require.Equal(t, "203.0.113.9", got.Forwarded)
	require.Equal(t, "10.1.2.3:4444", got.Remote)
}

func Test_Run_LeavesWaitOutWhenFalse(t *testing.T) {
	got := echoOf(t, invoke(t, newEchoEngine(), queueCall, `{"env":"dev","name":"q","wait":false}`))

	require.Equal(t, "/api/v1/queues/dev/q", got.URI)
	require.JSONEq(t, `{}`, got.Body)
}

func Test_FillPath_EscapesEachArgument(t *testing.T) {
	path, err := fillPath("/queues/:env/:name", map[string]any{"env": "dev", "name": "a b?c"})

	require.Nil(t, err)
	require.Equal(t, "/queues/dev/a%20b%3Fc", path)
}

func Test_FillPath_RefusesASlashThatWouldReachAnotherRoute(t *testing.T) {
	_, err := fillPath("/queues/:env/:name", map[string]any{"env": "dev", "name": "a/../../x"})

	require.NotNil(t, err)
	require.Equal(t, "name", err.Field)
}

func Test_Run_SendsAWholeBodyAsItIs(t *testing.T) {
	raw := call{method: http.MethodPut, path: "/raw", input: input{Body: bodyWhole, ContentType: "application/json"}}

	got := echoOf(t, invoke(t, newEchoEngine(), raw, `{"body":"plain"}`))

	require.Equal(t, `"plain"`, got.Body)
}

func Test_Run_NamesAMissingPathArgument(t *testing.T) {
	r := invoke(t, newEchoEngine(), queueCall, `{"env":"dev"}`)

	require.True(t, r.IsError)
	require.Contains(t, textOf(r, 0), `"field":"name"`)
}

func Test_Run_RefusesAnUnknownArgument(t *testing.T) {
	r := invoke(t, newEchoEngine(), queueCall, `{"env":"dev","name":"q","colour":"red"}`)

	require.True(t, r.IsError)
	require.Contains(t, textOf(r, 0), "colour")
}

func Test_Run_PassesAnAPIErrorOnWithItsHint(t *testing.T) {
	missing := call{method: http.MethodGet, path: "/missing"}

	r := invoke(t, newEchoEngine(), missing, `{}`)

	require.True(t, r.IsError)
	require.Contains(t, textOf(r, 0), `"hint":"list things first"`)
}

func Test_Run_SaysWhenAWaitEndedFirst(t *testing.T) {
	get := call{method: http.MethodGet, path: "/deployments/:id", input: input{Path: []string{"id"}, Query: []string{"wait"}}}

	r := invoke(t, newEchoEngine(), get, `{"id":"d1","wait":true}`)

	require.False(t, r.IsError)
	require.Len(t, r.Content, 2)
	require.Equal(t, notFinal, textOf(r, 1))
}

func Test_Run_PurgesOnlyWhenConfirmedByName(t *testing.T) {
	e := newEchoEngine()

	wrong := invoke(t, e, purgeCall, `{"env":"dev","name":"jobs","confirm":"job"}`)
	missing := invoke(t, e, purgeCall, `{"env":"dev","name":"jobs"}`)
	right := invoke(t, e, purgeCall, `{"env":"dev","name":"jobs","confirm":"jobs"}`)

	require.True(t, wrong.IsError)
	require.True(t, missing.IsError)
	require.False(t, right.IsError)
	require.Equal(t, "Done.", textOf(right, 0))
	require.Equal(t, 1, e.purges)
}

func Test_Arguments_TreatsNoArgumentsAsEmpty(t *testing.T) {
	args, err := arguments(nil)

	require.NoError(t, err)
	require.Empty(t, args)
}
