package services

import (
	"context"
	"net/http"

	"github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/scope"
	"github.com/gin-gonic/gin"
)

type Services interface {
	ListFor(ctx context.Context, a actor.Actor, teamSlug string) ([]entities.Service, error)
	GetFor(ctx context.Context, a actor.Actor, slug string) (entities.Service, error)
	Create(ctx context.Context, a actor.Actor, n entities.NewService) (entities.Service, error)
	Update(ctx context.Context, a actor.Actor, slug string, c entities.ServiceChanges) (entities.Service, error)
	Transfer(ctx context.Context, a actor.Actor, slug, teamSlug string) (entities.Service, error)
}

type Decommissioner interface {
	Start(ctx context.Context, a actor.Actor, slug string) (entities.Decommission, error)
}

type ServicesHandler struct {
	services     Services
	decommission Decommissioner
}

func NewServicesHandler(services Services, decommission Decommissioner) ServicesHandler {
	return ServicesHandler{services: services, decommission: decommission}
}

func (h ServicesHandler) List(c *gin.Context) {
	load := func() ([]entities.Service, error) {
		return h.services.ListFor(c.Request.Context(), requestctx.ActorFrom(c), c.Query("team"))
	}
	apispec.RespondWindow(c, load, serviceKey, ToDTO)
}

func serviceKey(s entities.Service) string {
	return s.Slug
}

func (h ServicesHandler) Get(c *gin.Context) {
	service, err := h.services.GetFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"))
	apispec.Respond(c, http.StatusOK, ToDTO(service), err)
}

func (h ServicesHandler) Create(c *gin.Context) {
	var body createDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	service, err := h.services.Create(c.Request.Context(), requestctx.ActorFrom(c), body.toEntity())
	apispec.Respond(c, http.StatusCreated, ToDTO(service), err)
}

func (h ServicesHandler) Update(c *gin.Context) {
	var body updateDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	service, err := h.services.Update(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), body.toEntity())
	apispec.Respond(c, http.StatusOK, ToDTO(service), err)
}

func (h ServicesHandler) Transfer(c *gin.Context) {
	var body transferDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	service, err := h.services.Transfer(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), body.Team)
	apispec.Respond(c, http.StatusOK, ToDTO(service), err)
}

func (h ServicesHandler) Delete(c *gin.Context) {
	result, err := h.decommission.Start(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"))
	apispec.Respond(c, http.StatusAccepted, toDecommissionDTO(result), err)
}

func RegisterServicesRoutes(api httpserver.API, h ServicesHandler) {
	api.Handle(
		apispec.Operation{ID: "listServices", Method: http.MethodGet, Path: "/services", Summary: "List the services you can see, by slug",
			Scope: scope.Read, Params: listParams{}, Status: http.StatusOK, Response: apispec.Page[ServiceDTO]{}, Handler: h.List},
		apispec.Operation{ID: "createService", Method: http.MethodPost, Path: "/services", Summary: "Register a service for a team",
			Scope: scope.Services, Body: createDTO{}, Status: http.StatusCreated, Response: ServiceDTO{}, Handler: h.Create},
		apispec.Operation{ID: "getService", Method: http.MethodGet, Path: "/services/:service", Summary: "Show a service",
			Scope: scope.Read, Params: serviceParams{}, Status: http.StatusOK, Response: ServiceDTO{}, Handler: h.Get},
		apispec.Operation{ID: "updateService", Method: http.MethodPatch, Path: "/services/:service", Summary: "Change a service's settings; fields left out stay",
			Scope: scope.Services, Params: serviceParams{}, Body: updateDTO{}, Status: http.StatusOK, Response: ServiceDTO{}, Handler: h.Update},
		apispec.Operation{ID: "deleteService", Method: http.MethodDelete, Path: "/services/:service",
			Summary: "Delete a service: freeze it, remove it from every environment, then drop it; call again to resume",
			Scope:   scope.ServicesAdmin, Params: serviceParams{}, Status: http.StatusAccepted, Response: decommissionDTO{}, Handler: h.Delete},
		apispec.Operation{ID: "transferService", Method: http.MethodPost, Path: "/services/:service/transfer", Summary: "Give a service to another team",
			Scope: scope.ServicesAdmin, Params: serviceParams{}, Body: transferDTO{}, Status: http.StatusOK, Response: ServiceDTO{}, Handler: h.Transfer},
	)
}
