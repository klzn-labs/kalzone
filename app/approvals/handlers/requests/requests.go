package requests

import (
	"context"
	"net/http"
	"strings"

	"github.com/adrianozp/platform/app/approvals/entities"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/gin-gonic/gin"
)

type Requests interface {
	Get(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
	List(ctx context.Context, a actor.Actor, status entities.Status) ([]entities.Request, error)
	Cancel(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
}

type RequestsHandler struct {
	requests Requests
	baseURL  string
}

func NewRequestsHandler(cfg config.Config, requests Requests) RequestsHandler {
	return RequestsHandler{requests: requests, baseURL: strings.TrimSuffix(cfg.BaseURL, "/")}
}

func (h RequestsHandler) List(c *gin.Context) {
	list, err := h.requests.List(c.Request.Context(), requestctx.ActorFrom(c), entities.Status(c.Query("status")))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]requestDTO, 0, len(list))
	for _, r := range list {
		result = append(result, toDTO(r, h.baseURL))
	}
	c.JSON(http.StatusOK, gin.H{"requests": result})
}

func (h RequestsHandler) Get(c *gin.Context) {
	r, err := h.requests.Get(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	h.respond(c, r, err)
}

func (h RequestsHandler) Cancel(c *gin.Context) {
	r, err := h.requests.Cancel(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	h.respond(c, r, err)
}

func (h RequestsHandler) respond(c *gin.Context, r entities.Request, err error) {
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, toDTO(r, h.baseURL))
}

func RegisterRequestsRoutes(api httpserver.API, h RequestsHandler) {
	api.GET("/requests", h.List)
	api.GET("/requests/:id", h.Get)
	api.POST("/requests/:id/cancel", h.Cancel)
}
