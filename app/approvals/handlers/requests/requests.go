package requests

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/httpserver/wait"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/klzn-labs/kalzone/pkg/page"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

type Requests interface {
	Get(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
	PageFor(ctx context.Context, a actor.Actor, status string, p page.Request) (page.Page[entities.Request], error)
	Cancel(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
	Preview(ctx context.Context, r entities.Request) (approval.Preview, error)
}

type RequestsHandler struct {
	requests Requests
	waiter   wait.Waiter
	baseURL  string
}

func NewRequestsHandler(cfg config.Config, requests Requests, waiter wait.Waiter) RequestsHandler {
	return RequestsHandler{requests: requests, waiter: waiter, baseURL: strings.TrimSuffix(cfg.BaseURL, "/")}
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
	d, ok := wait.Duration(c)
	if !ok {
		return
	}
	a, id := requestctx.ActorFrom(c), c.Param("id")
	load := func(ctx context.Context) (requestDTO, error) {
		r, err := h.requests.Get(ctx, a, id)
		if err != nil {
			return requestDTO{}, err
		}
		dto := h.toDTO(r)
		dto.Preview = h.previewOf(ctx, r)
		return dto, nil
	}
	wait.Read(c, h.waiter, d, load, finalRequest)
}

func finalRequest(dto requestDTO) bool {
	return entities.Request{Status: entities.Status(dto.Status)}.Final()
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
			Scope: scope.Read, Waits: true, Params: requestParams{}, Status: http.StatusOK, Response: requestDTO{}, Handler: h.Get},
		apispec.Operation{ID: "cancelRequest", Method: http.MethodPost, Path: "/requests/:id/cancel", Summary: "Cancel a pending change request you asked for",
			Params: requestParams{}, Status: http.StatusOK, Response: requestDTO{}, Handler: h.Cancel},
	)
}
