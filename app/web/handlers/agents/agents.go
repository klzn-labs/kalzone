package agents

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

const healthyWithin = time.Minute

type Agents interface {
	Agents(ctx context.Context) ([]deploy.Agent, error)
}

type Row struct {
	Agent   deploy.Agent
	Healthy bool
}

type AgentsHandler struct {
	renderer *web.Renderer
	agents   Agents
}

func NewAgentsHandler(renderer *web.Renderer, agents Agents) AgentsHandler {
	return AgentsHandler{renderer: renderer, agents: agents}
}

func (h AgentsHandler) Show(c *gin.Context) {
	if !requestctx.ActorFrom(c).Admin {
		web.ErrorPage(c, h.renderer, web.ErrAdminsOnly)
		return
	}
	list, err := h.agents.Agents(c.Request.Context())
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	rows := make([]Row, 0, len(list))
	for _, a := range list {
		rows = append(rows, Row{Agent: a, Healthy: clock.Now().Sub(a.LastSeenAt) < healthyWithin})
	}
	h.renderer.Render(c, http.StatusOK, "agents", web.Page{Title: "Agents", Nav: "agents", Data: rows})
}

func RegisterAgentsRoutes(routes httpserver.Web, h AgentsHandler) {
	routes.GET("/agents", h.Show)
}
