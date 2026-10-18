package deployments

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/deploy/usecases/deployments"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/httpserver/wait"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/page"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

type Deployments interface {
	PageFor(ctx context.Context, a actor.Actor, serviceSlug, environment string, p page.Request) (page.Page[entities.Deployment], error)
	GetFor(ctx context.Context, a actor.Actor, id string) (entities.Deployment, error)
	SummaryFor(ctx context.Context, a actor.Actor, id string) (entities.Deployment, entities.Summary, error)
	EventsFor(ctx context.Context, a actor.Actor, id string, p page.Request, tail string) (page.Page[entities.Event], error)
	Deploy(ctx context.Context, a actor.Actor, r deployments.Request) (entities.Deployment, error)
	DeployAll(ctx context.Context, a actor.Actor, b deployments.Batch) ([]entities.Deployment, error)
	PromoteAll(ctx context.Context, a actor.Actor, targets []deployments.Target, from string) ([]entities.Deployment, error)
	ApplyConfig(ctx context.Context, a actor.Actor, serviceSlug, environment string) (entities.Deployment, error)
	Promote(ctx context.Context, a actor.Actor, serviceSlug, from, to string) (entities.Deployment, error)
	Rollback(ctx context.Context, a actor.Actor, deploymentID string) (entities.Deployment, error)
}

type DeploymentsHandler struct {
	deployments Deployments
	waiter      wait.Waiter
}

func NewDeploymentsHandler(deployments Deployments, waiter wait.Waiter) DeploymentsHandler {
	return DeploymentsHandler{deployments: deployments, waiter: waiter}
}

func (h DeploymentsHandler) List(c *gin.Context) {
	load := func(p page.Request) (page.Page[entities.Deployment], error) {
		return h.deployments.PageFor(c.Request.Context(), requestctx.ActorFrom(c), c.Query("service"), c.Query("env"), p)
	}
	apispec.RespondPage(c, load, ToDTO)
}

func (h DeploymentsHandler) Get(c *gin.Context) {
	d, ok := wait.Duration(c)
	if !ok {
		return
	}
	a, id := requestctx.ActorFrom(c), c.Param("id")
	summarized := func(ctx context.Context) (DeploymentDTO, error) {
		dep, summary, err := h.deployments.SummaryFor(ctx, a, id)
		return withSummary(dep, summary), err
	}
	wait.PollThenRead(c, h.waiter, d, h.reread(c, id), summarized, finalDeployment)
}

func (h DeploymentsHandler) Events(c *gin.Context) {
	load := func(p page.Request) (page.Page[entities.Event], error) {
		return h.deployments.EventsFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"), p, c.Query("tail"))
	}
	apispec.RespondPage(c, load, toEventDTO)
}

func (h DeploymentsHandler) Deploy(c *gin.Context) {
	d, ok := wait.Duration(c)
	var body deployDTO
	if !ok || !httperror.BindStrict(c, &body) {
		return
	}
	batch, err := body.batch()
	switch {
	case err != nil:
		httperror.Abort(c, err)
	case batch:
		created, err := h.deployments.DeployAll(c.Request.Context(), requestctx.ActorFrom(c), body.toBatch())
		h.startedAll(c, d, created, err)
	default:
		dep, err := h.deployments.Deploy(c.Request.Context(), requestctx.ActorFrom(c), body.toRequest())
		h.started(c, d, dep, err)
	}
}

func (h DeploymentsHandler) Promote(c *gin.Context) {
	d, ok := wait.Duration(c)
	var body promoteDTO
	if !ok || !httperror.BindStrict(c, &body) {
		return
	}
	batch, err := body.batch()
	switch {
	case err != nil:
		httperror.Abort(c, err)
	case batch:
		created, err := h.deployments.PromoteAll(c.Request.Context(), requestctx.ActorFrom(c), toTargets(body.Targets), body.From)
		h.startedAll(c, d, created, err)
	default:
		dep, err := h.deployments.Promote(c.Request.Context(), requestctx.ActorFrom(c), body.Service, body.From, body.To)
		h.started(c, d, dep, err)
	}
}

func (h DeploymentsHandler) ApplyConfig(c *gin.Context) {
	d, ok := wait.Duration(c)
	var body applyDTO
	if !ok || !httperror.BindStrict(c, &body) {
		return
	}
	dep, err := h.deployments.ApplyConfig(c.Request.Context(), requestctx.ActorFrom(c), body.Service, body.Environment)
	h.started(c, d, dep, err)
}

func (h DeploymentsHandler) Rollback(c *gin.Context) {
	d, ok := wait.Duration(c)
	if !ok {
		return
	}
	dep, err := h.deployments.Rollback(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	h.started(c, d, dep, err)
}

func (h DeploymentsHandler) started(c *gin.Context, d time.Duration, dep entities.Deployment, err error) {
	wait.Started(c, h.waiter, d, ToDTO(dep), err, h.reread(c, dep.ID), finalDeployment)
}

func (h DeploymentsHandler) reread(c *gin.Context, id string) wait.Load[DeploymentDTO] {
	a := requestctx.ActorFrom(c)
	return func(ctx context.Context) (DeploymentDTO, error) {
		d, err := h.deployments.GetFor(ctx, a, id)
		return ToDTO(d), err
	}
}

func (h DeploymentsHandler) startedAll(c *gin.Context, d time.Duration, created []entities.Deployment, err error) {
	a := requestctx.ActorFrom(c)
	load := func(ctx context.Context) (apispec.Page[DeploymentDTO], error) {
		return h.current(ctx, a, created)
	}
	wait.Started(c, h.waiter, d, apispec.Items(page.Page[entities.Deployment]{Items: created}, ToDTO), err, load, allFinal)
}

func (h DeploymentsHandler) current(ctx context.Context, a actor.Actor, created []entities.Deployment) (apispec.Page[DeploymentDTO], error) {
	items := make([]DeploymentDTO, 0, len(created))
	for _, dep := range created {
		fresh, err := h.deployments.GetFor(ctx, a, dep.ID)
		if err != nil {
			return apispec.Page[DeploymentDTO]{}, err
		}
		items = append(items, ToDTO(fresh))
	}
	return apispec.Page[DeploymentDTO]{Items: items}, nil
}

func allFinal(p apispec.Page[DeploymentDTO]) bool {
	for _, dto := range p.Items {
		if !finalDeployment(dto) {
			return false
		}
	}
	return true
}

func finalDeployment(dto DeploymentDTO) bool {
	return !entities.Deployment{Status: entities.Status(dto.Status)}.Active()
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
			Scope: scope.Read, Waits: true, Params: deploymentParams{}, Status: http.StatusOK, Response: DeploymentDTO{}, Handler: h.Get},
		{ID: "listDeploymentEvents", Method: http.MethodGet, Path: "/deployments/:id/events", Summary: "Read a deployment's full log, oldest first; next_cursor stays while it runs",
			Scope: scope.Read, Params: eventsParams{}, Status: http.StatusOK, Response: apispec.Page[eventDTO]{}, Handler: h.Events},
	}
}

var (
	accepted      = apispec.OrChangeRequest[DeploymentDTO]{}
	acceptedBatch = apispec.BatchOrChangeRequest[DeploymentDTO]{}
)

func changeOperations(h DeploymentsHandler) []apispec.Operation {
	return []apispec.Operation{
		{ID: "createDeployment", Method: http.MethodPost, Path: "/deployments", Summary: "Deploy a built release to one environment, or to up to 10 targets all or nothing",
			Scope: scope.Deploy, Gated: true, Waits: true, Body: deployDTO{}, Status: http.StatusAccepted, Response: acceptedBatch, Handler: h.Deploy},
		{ID: "promoteDeployment", Method: http.MethodPost, Path: "/deployments/promotions", Summary: "Deploy what runs in one environment to another, for one service or up to 10 targets",
			Scope: scope.Deploy, Gated: true, Waits: true, Body: promoteDTO{}, Status: http.StatusAccepted, Response: acceptedBatch, Handler: h.Promote},
		{ID: "applyConfig", Method: http.MethodPost, Path: "/deployments/config-applications", Summary: "Redeploy the live release with the current config",
			Scope: scope.Deploy, Gated: true, Waits: true, Body: applyDTO{}, Status: http.StatusAccepted, Response: accepted, Handler: h.ApplyConfig},
		{ID: "rollbackDeployment", Method: http.MethodPost, Path: "/deployments/:id/rollback", Summary: "Deploy an earlier succeeded deployment's release and config again",
			Scope: scope.Deploy, Gated: true, Waits: true, Params: deploymentParams{}, Status: http.StatusAccepted, Response: accepted, Handler: h.Rollback},
	}
}
