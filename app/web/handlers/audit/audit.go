package audit

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	entities "github.com/klzn-labs/kalzone/app/audit/entities"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

const pageSize = 30

type Events interface {
	ListFor(ctx context.Context, a actor.Actor, filter entities.Filter) ([]entities.Event, error)
}

type FilterResolver interface {
	Resolve(ctx context.Context, query entities.Query) (entities.Filter, error)
}

type Teams interface {
	ListFor(ctx context.Context, a actor.Actor) ([]team.Team, error)
}

type Services interface {
	ListFor(ctx context.Context, a actor.Actor, teamSlug string) ([]catalog.Service, error)
}

type data struct {
	Query    entities.Query
	Activity []web.Activity
	Older    string
	Teams    []team.Team
	Services []catalog.Service
	Envs     []catalog.Environment
}

type AuditHandler struct {
	renderer *web.Renderer
	views    web.Views
	events   Events
	resolver FilterResolver
	teams    Teams
	services Services
}

func NewAuditHandler(renderer *web.Renderer, views web.Views, events Events, resolver FilterResolver, teams Teams, services Services) AuditHandler {
	return AuditHandler{renderer: renderer, views: views, events: events, resolver: resolver, teams: teams, services: services}
}

func (h AuditHandler) Show(c *gin.Context) {
	d, err := h.build(c.Request.Context(), requestctx.ActorFrom(c), queryFrom(c))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "audit", web.Page{Title: "Audit log", Nav: "audit", Data: d})
}

func (h AuditHandler) build(ctx context.Context, a actor.Actor, q entities.Query) (data, error) {
	d, err := h.options(ctx, a)
	if err != nil {
		return data{}, err
	}
	d.Query = q
	filter, err := h.resolver.Resolve(ctx, q)
	if err != nil {
		return d, nil
	}
	events, err := h.events.ListFor(ctx, a, filter)
	d.Activity = web.ActivityFrom(events)
	if len(events) == pageSize {
		d.Older = events[len(events)-1].At.Format(time.RFC3339Nano)
	}
	return d, err
}

func (h AuditHandler) options(ctx context.Context, a actor.Actor) (data, error) {
	teams, err := h.teams.ListFor(ctx, a)
	if err != nil {
		return data{}, err
	}
	services, err := h.services.ListFor(ctx, a, "")
	if err != nil {
		return data{}, err
	}
	envs, err := h.views.Environments(ctx)
	return data{Teams: teams, Services: services, Envs: envs}, err
}

func queryFrom(c *gin.Context) entities.Query {
	q := entities.Query{Team: c.Query("team"), Service: c.Query("service"), Actor: c.Query("actor"), Token: c.Query("token"), Environment: c.Query("env"), Limit: pageSize}
	if before, err := time.Parse(time.RFC3339Nano, c.Query("before")); err == nil {
		q.Before = &before
	}
	return q
}

func RegisterAuditRoutes(routes httpserver.Web, h AuditHandler) {
	routes.GET("/audit", h.Show)
}
