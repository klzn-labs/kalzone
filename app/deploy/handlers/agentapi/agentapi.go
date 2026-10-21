package agentapi

import (
	"context"
	"crypto/subtle"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/pkg/agentapi"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/config"
)

const agentKey = "platform.agent"

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,62}$`)

var (
	errNoAgents   = apperror.New(apperror.CodeNotConfigured, "agents are not configured on this control plane")
	errAgentToken = apperror.New(apperror.CodeUnauthenticated, "invalid agent token")
	errAgentName  = apperror.Invalid("X-Agent-Name must be a lowercase host name")
)

type Agents interface {
	Heartbeat(ctx context.Context, name, version string) error
	Claim(ctx context.Context, agent string) (entities.Workload, bool, error)
	Report(ctx context.Context, agent, deploymentID string, lines []string) error
	Finish(ctx context.Context, agent, deploymentID string, succeeded bool, message string, failure *entities.Failure) (entities.Deployment, error)
	Desired(ctx context.Context) ([]entities.Desired, error)
	Drift(ctx context.Context, agent string, drift entities.Drift) (entities.Deployment, bool, error)
	ReportStatus(ctx context.Context, agent string, at time.Time, snapshots []entities.Snapshot) error
	ReportPlatform(ctx context.Context, agent string, at time.Time, c entities.PlatformComponent) error
}

type AgentAPIHandler struct {
	token  []byte
	agents Agents
}

func NewAgentAPIHandler(cfg config.Config, agents Agents) AgentAPIHandler {
	return AgentAPIHandler{token: []byte(cfg.Agent.Token), agents: agents}
}

func (h AgentAPIHandler) authenticate(c *gin.Context) {
	if len(h.token) == 0 {
		httperror.Abort(c, errNoAgents)
		return
	}
	sent, _ := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(sent), h.token) != 1 {
		httperror.Abort(c, errAgentToken)
		return
	}
	name := c.GetHeader(agentapi.NameHeader)
	if !namePattern.MatchString(name) {
		httperror.Abort(c, errAgentName)
		return
	}
	if err := h.agents.Heartbeat(c.Request.Context(), name, c.GetHeader(agentapi.VersionHeader)); err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Set(agentKey, name)
}

func agentName(c *gin.Context) string {
	return c.GetString(agentKey)
}

func (h AgentAPIHandler) Claim(c *gin.Context) {
	w, claimed, err := h.agents.Claim(c.Request.Context(), agentName(c))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	if !claimed {
		c.Status(http.StatusNoContent)
		return
	}
	c.JSON(http.StatusOK, toWire(w))
}

func (h AgentAPIHandler) Events(c *gin.Context) {
	var body agentapi.Events
	if !httperror.BindJSON(c, &body) {
		return
	}
	if err := h.agents.Report(c.Request.Context(), agentName(c), c.Param("id"), body.Lines); err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h AgentAPIHandler) Finish(c *gin.Context) {
	var body agentapi.Finish
	if !httperror.BindJSON(c, &body) {
		return
	}
	if _, err := h.agents.Finish(c.Request.Context(), agentName(c), c.Param("id"), body.Succeeded, body.Message, failureOf(body.Failure)); err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h AgentAPIHandler) Desired(c *gin.Context) {
	desired, err := h.agents.Desired(c.Request.Context())
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	list := agentapi.DesiredList{Workloads: make([]agentapi.Desired, 0, len(desired))}
	for _, d := range desired {
		list.Workloads = append(list.Workloads, agentapi.Desired(d))
	}
	c.JSON(http.StatusOK, list)
}

func (h AgentAPIHandler) Drift(c *gin.Context) {
	var body agentapi.Drift
	if !httperror.BindJSON(c, &body) {
		return
	}
	_, created, err := h.agents.Drift(c.Request.Context(), agentName(c), entities.Drift(body))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"reconciling": created})
}

func toWire(w entities.Workload) agentapi.Workload {
	d, cfg := w.Deployment, w.Config
	return agentapi.Workload{
		DeploymentID: d.ID, Number: d.Number, Name: w.Name, Service: w.Service, Environment: d.Environment,
		ImageRef: d.ImageRef, Version: w.Version, Strategy: string(d.Strategy), Spec: agentapi.Spec(cfg.Spec),
		Env: cfg.Env, Secrets: cfg.Secrets,
	}
}

func RegisterAgentAPIRoutes(public httpserver.Public, h AgentAPIHandler) {
	group := public.Group("/agent/v1", h.authenticate)
	group.POST("/claim", h.Claim)
	group.POST("/deployments/:id/events", h.Events)
	group.POST("/deployments/:id/finish", h.Finish)
	group.GET("/desired", h.Desired)
	group.POST("/drift", h.Drift)
	group.POST("/status", h.Status)
	group.PUT("/platform/:name", h.Platform)
}
