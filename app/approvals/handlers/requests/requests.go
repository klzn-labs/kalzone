package requests

import (
	"context"
	"net/http"
	"strings"

	"github.com/adrianozp/platform/app/approvals/entities"
	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/adrianozp/platform/pkg/page"
	"github.com/adrianozp/platform/pkg/scope"
	"github.com/gin-gonic/gin"
)

type Requests interface {
	Get(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
	PageFor(ctx context.Context, a actor.Actor, status string, p page.Request) (page.Page[entities.Request], error)
	Cancel(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
}

type RequestsHandler struct {
	requests Requests
	baseURL  string
}

func NewRequestsHandler(cfg config.Config, requests Requests) RequestsHandler {
	return RequestsHandler{requests: requests, baseURL: strings.TrimSuffix(cfg.BaseURL, "/")}
}

type listParams struct {
	Status string `query:"status" enum:"pending,approved,executed,failed,rejected,cancelled,expired"`
	apispec.ListParams
}

type requestParams struct {
	ID string `path:"id"`
}

func (h RequestsHandler) List(c *gin.Context) {
	load := func(p page.Request) (page.Page[entities.Request], error) {
		return h.requests.PageFor(c.Request.Context(), requestctx.ActorFrom(c), c.Query("status"), p)
	}
	apispec.RespondPage(c, load, h.toDTO)
}

func (h RequestsHandler) Get(c *gin.Context) {
	r, err := h.requests.Get(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	apispec.Respond(c, http.StatusOK, h.toDTO(r), err)
}

func (h RequestsHandler) Cancel(c *gin.Context) {
	r, err := h.requests.Cancel(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	apispec.Respond(c, http.StatusOK, h.toDTO(r), err)
}

func (h RequestsHandler) toDTO(r entities.Request) requestDTO {
	return toDTO(r, h.baseURL)
}

func RegisterRequestsRoutes(api httpserver.API, h RequestsHandler) {
	api.Handle(
		apispec.Operation{ID: "listRequests", Method: http.MethodGet, Path: "/requests", Summary: "List change requests of your teams, newest first",
			Scope: scope.Read, Params: listParams{}, Status: http.StatusOK, Response: apispec.Page[requestDTO]{}, Handler: h.List},
		apispec.Operation{ID: "getRequest", Method: http.MethodGet, Path: "/requests/:id", Summary: "Show a change request and its outcome",
			Scope: scope.Read, Params: requestParams{}, Status: http.StatusOK, Response: requestDTO{}, Handler: h.Get},
		apispec.Operation{ID: "cancelRequest", Method: http.MethodPost, Path: "/requests/:id/cancel", Summary: "Cancel a pending change request you asked for",
			Params: requestParams{}, Status: http.StatusOK, Response: requestDTO{}, Handler: h.Cancel},
	)
}
