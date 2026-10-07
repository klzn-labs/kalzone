package service

import (
	"context"
	"net/http"

	audit "github.com/adrianozp/platform/app/audit/entities"
	"github.com/adrianozp/platform/app/authz"
	catalog "github.com/adrianozp/platform/app/catalog/entities"
	envconfig "github.com/adrianozp/platform/app/envconfig/entities"
	team "github.com/adrianozp/platform/app/team/entities"
	"github.com/adrianozp/platform/app/web"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/gin-gonic/gin"
)

type Services interface {
	Get(ctx context.Context, slug string) (catalog.Service, error)
	Update(ctx context.Context, a actor.Actor, slug string, c catalog.ServiceChanges) (catalog.Service, error)
	Transfer(ctx context.Context, a actor.Actor, slug, teamSlug string) (catalog.Service, error)
	Delete(ctx context.Context, a actor.Actor, slug string) error
}

type Teams interface {
	List(ctx context.Context) ([]team.Team, error)
}

type Configs interface {
	Current(ctx context.Context, serviceSlug, environment string) (envconfig.Revision, error)
}

type Events interface {
	List(ctx context.Context, filter audit.Filter) ([]audit.Event, error)
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
}

type ServiceHandler struct {
	renderer *web.Renderer
	views    web.Views
	services Services
	teams    Teams
	configs  Configs
	events   Events
}

func NewServiceHandler(renderer *web.Renderer, views web.Views, services Services, teams Teams, configs Configs, events Events) ServiceHandler {
	return ServiceHandler{renderer: renderer, views: views, services: services, teams: teams, configs: configs, events: events}
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
	service, err := h.services.Get(ctx, slug)
	if err != nil {
		return data{}, err
	}
	teams, err := h.teams.List(ctx)
	if err != nil {
		return data{}, err
	}
	envs, err := h.envSummaries(ctx, slug)
	if err != nil {
		return data{}, err
	}
	events, err := h.events.List(ctx, audit.Filter{TargetID: service.ID, Limit: 15})
	resource := authz.Resource{TeamID: service.OwnerTeamID}
	return data{
		Service: service, Team: owner(teams, service.OwnerTeamID), Teams: teams, Envs: envs, Activity: web.ActivityFrom(events),
		CanEdit: h.views.Can(ctx, a, authz.EditService, resource), CanMaintain: h.views.Can(ctx, a, authz.DeleteService, resource),
	}, err
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
