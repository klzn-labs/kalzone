package service

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	audit "github.com/klzn-labs/kalzone/app/audit/entities"
	"github.com/klzn-labs/kalzone/app/authz"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	envconfig "github.com/klzn-labs/kalzone/app/envconfig/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Services interface {
	GetFor(ctx context.Context, a actor.Actor, slug string) (catalog.Service, error)
	Update(ctx context.Context, a actor.Actor, slug string, c catalog.ServiceChanges) (catalog.Service, error)
	Transfer(ctx context.Context, a actor.Actor, slug, teamSlug string) (catalog.Service, error)
}

type Decommissioner interface {
	Start(ctx context.Context, a actor.Actor, slug string) (catalog.Decommission, error)
}

type Teams interface {
	ListFor(ctx context.Context, a actor.Actor) ([]team.Team, error)
}

type Configs interface {
	Current(ctx context.Context, serviceSlug, environment string) (envconfig.Revision, error)
}

type Events interface {
	ListFor(ctx context.Context, a actor.Actor, filter audit.Filter) ([]audit.Event, error)
}

type EnvSummary struct {
	Env      catalog.Environment
	Revision envconfig.Revision
}

type data struct {
	Service     catalog.Service
	Team        team.Team
	Teams       []team.Team
	Envs        []EnvSummary
	Activity    []web.Activity
	CanEdit     bool
	CanMaintain bool
	Resources   []ResourceUse
	Status      web.ServiceStatus
	Prod        EnvSummary
}

type ServiceHandler struct {
	renderer     *web.Renderer
	views        web.Views
	services     Services
	decommission Decommissioner
	teams        Teams
	configs      Configs
	events       Events
	queues       Queues
	databases    Databases
	statuses     web.Statuses
}

type Deps struct {
	Services     Services
	Decommission Decommissioner
	Teams        Teams
	Configs      Configs
	Events       Events
	Queues       Queues
	Databases    Databases
	Statuses     web.Statuses
}

func NewServiceHandler(renderer *web.Renderer, views web.Views, d Deps) ServiceHandler {
	return ServiceHandler{
		renderer: renderer, views: views, services: d.Services, decommission: d.Decommission,
		teams: d.Teams, configs: d.Configs, events: d.Events, queues: d.Queues, databases: d.Databases, statuses: d.Statuses,
	}
}

func (h ServiceHandler) Show(c *gin.Context) {
	d, err := h.build(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "service", web.Page{Title: d.Service.Name, Nav: "services", Data: d})
}

func (h ServiceHandler) build(ctx context.Context, a actor.Actor, slug string) (data, error) {
	service, err := h.services.GetFor(ctx, a, slug)
	if err != nil {
		return data{}, err
	}
	teams, err := h.teams.ListFor(ctx, a)
	if err != nil {
		return data{}, err
	}
	envs, err := h.envSummaries(ctx, slug)
	if err != nil {
		return data{}, err
	}
	d := data{Service: service, Team: owner(teams, service.OwnerTeamID), Teams: teams, Envs: envs}
	if d, err = h.withStatus(ctx, a, d); err != nil {
		return data{}, err
	}
	return h.withHistory(ctx, a, d)
}

func (h ServiceHandler) withStatus(ctx context.Context, a actor.Actor, d data) (data, error) {
	for _, env := range d.Envs {
		if env.Env.Protected && d.Prod.Env.Slug == "" {
			d.Prod = env
		}
	}
	var err error
	d.Status, err = h.statuses.Day(ctx, a, d.Service, d.Prod.Env.Slug)
	return d, err
}

func (h ServiceHandler) withHistory(ctx context.Context, a actor.Actor, d data) (data, error) {
	events, err := h.events.ListFor(ctx, a, audit.Filter{TargetID: d.Service.ID, Limit: 15})
	if err != nil {
		return data{}, err
	}
	d.Activity = web.ActivityFrom(events)
	d.Resources, err = h.resources(ctx, d.Service)
	resource := authz.Resource{TeamID: d.Service.OwnerTeamID}
	d.CanEdit = h.views.Can(ctx, a, authz.EditService, resource)
	d.CanMaintain = h.views.Can(ctx, a, authz.DeleteService, resource)
	return d, err
}

func (h ServiceHandler) envSummaries(ctx context.Context, slug string) ([]EnvSummary, error) {
	envs, err := h.views.Environments(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]EnvSummary, 0, len(envs))
	for _, env := range envs {
		rev, err := h.configs.Current(ctx, slug, env.Slug)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, EnvSummary{Env: env, Revision: rev})
	}
	return summaries, nil
}

func owner(teams []team.Team, id string) team.Team {
	for _, t := range teams {
		if t.ID == id {
			return t
		}
	}
	return team.Team{}
}

func RegisterServiceRoutes(routes httpserver.Web, h ServiceHandler) {
	routes.GET("/services/:service", h.Show)
	routes.POST("/services/:service/update", h.Update)
	routes.POST("/services/:service/transfer", h.Transfer)
	routes.POST("/services/:service/delete", h.Delete)
}
