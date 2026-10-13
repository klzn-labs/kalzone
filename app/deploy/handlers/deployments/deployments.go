package deployments

import (
	"context"
	"net/http"

	"github.com/adrianozp/platform/app/deploy/entities"
	"github.com/adrianozp/platform/app/deploy/usecases/deployments"
	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/page"
	"github.com/adrianozp/platform/pkg/scope"
	"github.com/gin-gonic/gin"
)

type Deployments interface {
	PageFor(ctx context.Context, a actor.Actor, serviceSlug, environment string, p page.Request) (page.Page[entities.Deployment], error)
	SummaryFor(ctx context.Context, a actor.Actor, id string) (entities.Deployment, entities.Summary, error)
	EventsFor(ctx context.Context, a actor.Actor, id string, p page.Request, tail string) (page.Page[entities.Event], error)
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
	load := func(p page.Request) (page.Page[entities.Deployment], error) {
		return h.deployments.PageFor(c.Request.Context(), requestctx.ActorFrom(c), c.Query("service"), c.Query("env"), p)
	}
	apispec.RespondPage(c, load, ToDTO)
}

func (h DeploymentsHandler) Get(c *gin.Context) {
	d, summary, err := h.deployments.SummaryFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	apispec.Respond(c, http.StatusOK, withSummary(d, summary), err)
}

func (h DeploymentsHandler) Events(c *gin.Context) {
	load := func(p page.Request) (page.Page[entities.Event], error) {
		return h.deployments.EventsFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"), p, c.Query("tail"))
	}
	apispec.RespondPage(c, load, toEventDTO)
}

func (h DeploymentsHandler) Deploy(c *gin.Context) {
	var body deployDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	d, err := h.deployments.Deploy(c.Request.Context(), requestctx.ActorFrom(c), body.toRequest())
	apispec.Respond(c, http.StatusAccepted, ToDTO(d), err)
}

func (h DeploymentsHandler) Promote(c *gin.Context) {
	var body promoteDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	d, err := h.deployments.Promote(c.Request.Context(), requestctx.ActorFrom(c), body.Service, body.From, body.To)
	apispec.Respond(c, http.StatusAccepted, ToDTO(d), err)
}

func (h DeploymentsHandler) ApplyConfig(c *gin.Context) {
	var body applyDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	d, err := h.deployments.ApplyConfig(c.Request.Context(), requestctx.ActorFrom(c), body.Service, body.Environment)
	apispec.Respond(c, http.StatusAccepted, ToDTO(d), err)
}

func (h DeploymentsHandler) Rollback(c *gin.Context) {
	d, err := h.deployments.Rollback(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	apispec.Respond(c, http.StatusAccepted, ToDTO(d), err)
}

func RegisterDeploymentsRoutes(api httpserver.API, h DeploymentsHandler) {
	api.Handle(readOperations(h)...)
	api.Handle(changeOperations(h)...)
}

func readOperations(h DeploymentsHandler) []apispec.Operation {
	return []apispec.Operation{
		{ID: "listDeployments", Method: http.MethodGet, Path: "/deployments", Summary: "List a service's deployments to one environment, newest first",
			Scope: scope.Read, Params: listParams{}, Status: http.StatusOK, Response: apispec.Page[DeploymentDTO]{}, Handler: h.List},
		{ID: "getDeployment", Method: http.MethodGet, Path: "/deployments/:id", Summary: "Show a deployment with a summary of its log",
			Scope: scope.Read, Params: deploymentParams{}, Status: http.StatusOK, Response: DeploymentDTO{}, Handler: h.Get},
		{ID: "listDeploymentEvents", Method: http.MethodGet, Path: "/deployments/:id/events", Summary: "Read a deployment's full log, oldest first; next_cursor stays while it runs",
			Scope: scope.Read, Params: eventsParams{}, Status: http.StatusOK, Response: apispec.Page[eventDTO]{}, Handler: h.Events},
	}
}

var accepted = apispec.OrChangeRequest[DeploymentDTO]{}

func changeOperations(h DeploymentsHandler) []apispec.Operation {
	return []apispec.Operation{
		{ID: "createDeployment", Method: http.MethodPost, Path: "/deployments", Summary: "Deploy a built release to an environment",
			Scope: scope.Deploy, Gated: true, Body: deployDTO{}, Status: http.StatusAccepted, Response: accepted, Handler: h.Deploy},
		{ID: "promoteDeployment", Method: http.MethodPost, Path: "/deployments/promotions", Summary: "Deploy what runs in one environment to another",
			Scope: scope.Deploy, Gated: true, Body: promoteDTO{}, Status: http.StatusAccepted, Response: accepted, Handler: h.Promote},
		{ID: "applyConfig", Method: http.MethodPost, Path: "/deployments/config-applications", Summary: "Redeploy the live release with the current config",
			Scope: scope.Deploy, Gated: true, Body: applyDTO{}, Status: http.StatusAccepted, Response: accepted, Handler: h.ApplyConfig},
		{ID: "rollbackDeployment", Method: http.MethodPost, Path: "/deployments/:id/rollback", Summary: "Deploy an earlier succeeded deployment's release and config again",
			Scope: scope.Deploy, Gated: true, Params: deploymentParams{}, Status: http.StatusAccepted, Response: accepted, Handler: h.Rollback},
	}
}
