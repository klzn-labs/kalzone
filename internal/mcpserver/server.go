package mcpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const instructions = "Kalzone, the internal developer platform: services, their config and secrets, releases, deployments, queues and change requests. " +
	"Every tool acts as the token's owner, limited to the token's scopes and teams. " +
	"On a protected environment such as prod a change answers a change request: a human approves it at approve_url, and the wait tool with kind request follows it. " +
	"Builds and deployments take wait: true to answer once they are final, and the wait tool waits on a release, deployment or change request by id. " +
	"An error carries a hint that says how to fix the call."

var options = &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}

type Handler struct {
	engine http.Handler
	tools  []Tool
}

func (h *Handler) load(catalog *apispec.Catalog, baseURL string) error {
	raw, err := catalog.Document(baseURL)
	if err != nil {
		return err
	}
	tools, err := Build(catalog, raw)
	h.tools = append(tools, waitTool())
	return err
}

func (h *Handler) Serve(c *gin.Context) {
	server := h.serverFor(c.Request, requestctx.ActorFrom(c))
	mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, options).ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) serverFor(origin *http.Request, a actor.Actor) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "kalzone", Version: "1"}, &mcp.ServerOptions{Instructions: instructions})
	for _, tool := range h.tools {
		if tool.VisibleTo(a) {
			server.AddTool(tool.Definition, tool.handler(h.engine, origin))
		}
	}
	return server
}
