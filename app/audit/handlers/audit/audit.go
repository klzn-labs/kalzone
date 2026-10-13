package audit

import (
	"context"
	"net/http"
	"time"

	"github.com/adrianozp/platform/app/audit/entities"
	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/page"
	"github.com/adrianozp/platform/pkg/scope"
	"github.com/gin-gonic/gin"
)

type Events interface {
	PageFor(ctx context.Context, a actor.Actor, filter entities.Filter, p page.Request) (page.Page[entities.Event], error)
}

type FilterResolver interface {
	Resolve(ctx context.Context, query entities.Query) (entities.Filter, error)
}

type AuditHandler struct {
	events   Events
	resolver FilterResolver
}

func NewAuditHandler(events Events, resolver FilterResolver) AuditHandler {
	return AuditHandler{events: events, resolver: resolver}
}

type eventDTO struct {
	ID          string         `json:"id"`
	At          time.Time      `json:"at"`
	Actor       string         `json:"actor"`
	Action      string         `json:"action"`
	TargetKind  string         `json:"target_kind"`
	TargetName  string         `json:"target_name"`
	Environment string         `json:"environment,omitempty"`
	Details     map[string]any `json:"details"`
	Token       string         `json:"token,omitempty"`
}

func toDTO(e entities.Event) eventDTO {
	return eventDTO{e.ID, e.At, e.ActorLogin, e.Action, e.TargetKind, e.TargetName, e.Environment, e.Details, e.TokenName}
}

type listParams struct {
	Team        string `query:"team" description:"team slug"`
	Service     string `query:"service" description:"service slug"`
	Actor       string `query:"actor" description:"login of the person who acted"`
	Environment string `query:"env" description:"environment slug"`
	Token       string `query:"token" description:"token id, from GET /api/v1/tokens"`
	apispec.ListParams
}

func (h AuditHandler) List(c *gin.Context) {
	apispec.RespondPage(c, func(p page.Request) (page.Page[entities.Event], error) { return h.list(c, p) }, toDTO)
}

func (h AuditHandler) list(c *gin.Context, p page.Request) (page.Page[entities.Event], error) {
	filter, err := h.resolver.Resolve(c.Request.Context(), queryFrom(c))
	if err != nil {
		return page.Page[entities.Event]{}, err
	}
	return h.events.PageFor(c.Request.Context(), requestctx.ActorFrom(c), filter, p)
}

func queryFrom(c *gin.Context) entities.Query {
	return entities.Query{Team: c.Query("team"), Service: c.Query("service"), Actor: c.Query("actor"), Environment: c.Query("env"), Token: c.Query("token")}
}

func RegisterAuditRoutes(api httpserver.API, h AuditHandler) {
	api.Handle(apispec.Operation{
		ID: "listAuditEvents", Method: http.MethodGet, Path: "/audit", Summary: "List what happened, newest first, limited to the token's teams",
		Scope: scope.Read, Params: listParams{}, Status: http.StatusOK, Response: apispec.Page[eventDTO]{}, Handler: h.List,
	})
}
