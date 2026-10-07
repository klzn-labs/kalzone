package services

import (
	"context"
	"net/http"

	"github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/gin-gonic/gin"
)

type Services interface {
	List(ctx context.Context, filter entities.ServiceFilter) ([]entities.Service, error)
	ListOwnedBy(ctx context.Context, teamSlug string) ([]entities.Service, error)
	Get(ctx context.Context, slug string) (entities.Service, error)
	Create(ctx context.Context, a actor.Actor, n entities.NewService) (entities.Service, error)
	Update(ctx context.Context, a actor.Actor, slug string, c entities.ServiceChanges) (entities.Service, error)
	Transfer(ctx context.Context, a actor.Actor, slug, teamSlug string) (entities.Service, error)
	Delete(ctx context.Context, a actor.Actor, slug string) error
}

type ServicesHandler struct {
	services Services
}

func NewServicesHandler(services Services) ServicesHandler {
	return ServicesHandler{services: services}
}

func (h ServicesHandler) List(c *gin.Context) {
	list, err := h.list(c)
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]ServiceDTO, 0, len(list))
	for _, s := range list {
		result = append(result, ToDTO(s))
	}
	c.JSON(http.StatusOK, gin.H{"services": result})
}

func (h ServicesHandler) list(c *gin.Context) ([]entities.Service, error) {
	if team := c.Query("team"); team != "" {
		return h.services.ListOwnedBy(c.Request.Context(), team)
	}
	return h.services.List(c.Request.Context(), entities.ServiceFilter{})
}

func (h ServicesHandler) Get(c *gin.Context) {
	service, err := h.services.Get(c.Request.Context(), c.Param("service"))
	respond(c, http.StatusOK, service, err)
}

func (h ServicesHandler) Create(c *gin.Context) {
	var body createDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	service, err := h.services.Create(c.Request.Context(), requestctx.ActorFrom(c), body.toEntity())
	respond(c, http.StatusCreated, service, err)
}

func (h ServicesHandler) Update(c *gin.Context) {
	var body updateDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	service, err := h.services.Update(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), body.toEntity())
	respond(c, http.StatusOK, service, err)
}

func (h ServicesHandler) Transfer(c *gin.Context) {
	var body transferDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	service, err := h.services.Transfer(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), body.Team)
	respond(c, http.StatusOK, service, err)
}

func (h ServicesHandler) Delete(c *gin.Context) {
	if err := h.services.Delete(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service")); err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func respond(c *gin.Context, status int, service entities.Service, err error) {
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(status, ToDTO(service))
}

func RegisterServicesRoutes(api httpserver.API, h ServicesHandler) {
	api.GET("/services", h.List)
	api.POST("/services", h.Create)
	api.GET("/services/:service", h.Get)
	api.PATCH("/services/:service", h.Update)
	api.DELETE("/services/:service", h.Delete)
	api.POST("/services/:service/transfer", h.Transfer)
}
