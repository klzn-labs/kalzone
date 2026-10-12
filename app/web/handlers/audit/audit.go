package audit

import (
	"context"
	"net/http"
	"time"

	entities "github.com/adrianozp/platform/app/audit/entities"
	catalog "github.com/adrianozp/platform/app/catalog/entities"
	team "github.com/adrianozp/platform/app/team/entities"
	"github.com/adrianozp/platform/app/web"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/gin-gonic/gin"
)

const pageSize = 30

type Events interface {
	List(ctx context.Context, filter entities.Filter) ([]entities.Event, error)
}

type FilterResolver interface {
	Resolve(ctx context.Context, query entities.Query) (entities.Filter, error)
}

type Teams interface {
	List(ctx context.Context) ([]team.Team, error)
}

type Services interface {
	List(ctx context.Context, filter catalog.ServiceFilter) ([]catalog.Service, error)
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
	d, err := h.build(c.Request.Context(), queryFrom(c))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "audit", web.Page{Title: "Audit log", Nav: "audit", Data: d})
}

func (h AuditHandler) build(ctx context.Context, q entities.Query) (data, error) {
	d, err := h.options(ctx)
	if err != nil {
		return data{}, err
	}
	d.Query = q
	filter, err := h.resolver.Resolve(ctx, q)
	if err != nil {
		return d, nil
	}
	events, err := h.events.List(ctx, filter)
	d.Activity = web.ActivityFrom(events)
	if len(events) == pageSize {
		d.Older = events[len(events)-1].At.Format(time.RFC3339Nano)
	}
	return d, err
}

func (h AuditHandler) options(ctx context.Context) (data, error) {
	teams, err := h.teams.List(ctx)
	if err != nil {
		return data{}, err
	}
	services, err := h.services.List(ctx, catalog.ServiceFilter{})
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
