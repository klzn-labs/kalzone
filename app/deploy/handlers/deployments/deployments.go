package deployments

import (
	"context"
	"net/http"
	"strconv"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/app/deploy/usecases/deployments"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/gin-gonic/gin"
)

type Deployments interface {
	History(ctx context.Context, serviceSlug, environment string, limit int) ([]entities.Deployment, error)
	Get(ctx context.Context, id string) (entities.Deployment, []entities.Event, error)
	Deploy(ctx context.Context, a actor.Actor, r deployments.Request) (entities.Deployment, error)
	ApplyConfig(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.Deployment, error)
	Promote(ctx context.Context, a actor.Actor, serviceSlug, from, to string) (entities.Deployment, error)
	Rollback(ctx context.Context, a actor.Actor, deploymentID string) (entities.Deployment, error)
}

type DeploymentsHandler struct {
	deployments Deployments
}

func NewDeploymentsHandler(deployments Deployments) DeploymentsHandler {
	return DeploymentsHandler{deployments: deployments}
}

func (h DeploymentsHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	list, err := h.deployments.History(c.Request.Context(), c.Query("service"), c.Query("env"), limit)
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]DeploymentDTO, 0, len(list))
	for _, d := range list {
		result = append(result, ToDTO(d))
	}
	c.JSON(http.StatusOK, gin.H{"deployments": result})
}

func (h DeploymentsHandler) Get(c *gin.Context) {
	d, logged, err := h.deployments.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	lines := make([]eventDTO, 0, len(logged))
	for _, e := range logged {
		lines = append(lines, eventDTO{At: e.At, Message: e.Message})
	}
	c.JSON(http.StatusOK, gin.H{"deployment": ToDTO(d), "events": lines})
}

func (h DeploymentsHandler) Deploy(c *gin.Context) {
	var body deployDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	d, err := h.deployments.Deploy(c.Request.Context(), requestctx.ActorFrom(c), deployments.Request(body))
	respond(c, d, err)
}

func (h DeploymentsHandler) Promote(c *gin.Context) {
	var body promoteDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	d, err := h.deployments.Promote(c.Request.Context(), requestctx.ActorFrom(c), body.Service, body.From, body.To)
	respond(c, d, err)
}

func (h DeploymentsHandler) ApplyConfig(c *gin.Context) {
	var body applyDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	d, err := h.deployments.ApplyConfig(c.Request.Context(), requestctx.ActorFrom(c), body.Service, body.Environment)
	respond(c, d, err)
}

func (h DeploymentsHandler) Rollback(c *gin.Context) {
	d, err := h.deployments.Rollback(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	respond(c, d, err)
}

func respond(c *gin.Context, d entities.Deployment, err error) {
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusAccepted, ToDTO(d))
}

func RegisterDeploymentsRoutes(api httpserver.API, h DeploymentsHandler) {
	api.GET("/deployments", h.List)
	api.POST("/deployments", h.Deploy)
	api.POST("/deployments/promotions", h.Promote)
	api.POST("/deployments/config-applications", h.ApplyConfig)
	api.GET("/deployments/:id", h.Get)
	api.POST("/deployments/:id/rollback", h.Rollback)
}
