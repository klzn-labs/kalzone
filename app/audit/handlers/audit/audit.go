package audit

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/adrianozp/platform/app/audit/entities"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/gin-gonic/gin"
)

type Events interface {
	ListFor(ctx context.Context, a actor.Actor, filter entities.Filter) ([]entities.Event, error)
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

func (h AuditHandler) List(c *gin.Context) {
	query, err := QueryFrom(c)
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	list, err := h.list(c.Request.Context(), requestctx.ActorFrom(c), query)
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]eventDTO, 0, len(list))
	for _, e := range list {
		result = append(result, eventDTO{e.ID, e.At, e.ActorLogin, e.Action, e.TargetKind, e.TargetName, e.Environment, e.Details, e.TokenName})
	}
	c.JSON(http.StatusOK, gin.H{"events": result})
}

func (h AuditHandler) list(ctx context.Context, a actor.Actor, query entities.Query) ([]entities.Event, error) {
	filter, err := h.resolver.Resolve(ctx, query)
	if err != nil {
		return nil, err
	}
	return h.events.ListFor(ctx, a, filter)
}

func QueryFrom(c *gin.Context) (entities.Query, error) {
	query := entities.Query{Team: c.Query("team"), Service: c.Query("service"), Actor: c.Query("actor"), Environment: c.Query("env"), Token: c.Query("token")}
	query.Limit, _ = strconv.Atoi(c.Query("limit"))
	if before := c.Query("before"); before != "" {
		at, err := time.Parse(time.RFC3339Nano, before)
		if err != nil {
			return entities.Query{}, apperror.Invalid("before must be an RFC 3339 time")
		}
		query.Before = &at
	}
	return query, nil
}

func RegisterAuditRoutes(api httpserver.API, h AuditHandler) {
	api.GET("/audit", h.List)
}
