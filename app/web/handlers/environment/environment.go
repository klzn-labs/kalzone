package environment

import (
	"context"
	"net/http"
	"sort"

	"github.com/adrianozp/platform/app/authz"
	catalog "github.com/adrianozp/platform/app/catalog/entities"
	envconfig "github.com/adrianozp/platform/app/envconfig/entities"
	identity "github.com/adrianozp/platform/app/identity/entities"
	team "github.com/adrianozp/platform/app/team/entities"
	"github.com/adrianozp/platform/app/web"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/gin-gonic/gin"
)

type Services interface {
	Get(ctx context.Context, slug string) (catalog.Service, error)
}

type Configs interface {
	Current(ctx context.Context, serviceSlug, environment string) (envconfig.Revision, error)
	History(ctx context.Context, serviceSlug, environment string, limit int) ([]envconfig.Revision, error)
	Secrets(ctx context.Context, serviceSlug, environment string) ([]envconfig.SecretInfo, error)
	Revise(ctx context.Context, a actor.Actor, serviceSlug, environment string, cs envconfig.ChangeSet) (envconfig.Revision, error)
	SetSecret(ctx context.Context, a actor.Actor, serviceSlug, environment, name, value string) (envconfig.Revision, error)
	UnsetSecret(ctx context.Context, a actor.Actor, serviceSlug, environment, name string) (envconfig.Revision, error)
}

type EnvTab struct {
	Env catalog.Environment
}

type Variable struct {
	Name  string
	Value string
}

type SecretRow struct {
	Name  string
	SetBy identity.User
	Info  envconfig.SecretInfo
}

type RevisionView struct {
	Revision envconfig.Revision
	Author   identity.User
	Changes  []envconfig.Change
}

type data struct {
	Service   catalog.Service
	Team      team.Team
	Envs      []EnvTab
	Env       catalog.Environment
	Revision  envconfig.Revision
	Variables []Variable
	Secrets   []SecretRow
	History   []RevisionView
	CanEdit   bool
	Deploy    DeployView
	Grafana   GrafanaView
}

type GrafanaView struct {
	Enabled bool
	UID     string
	Env     string
}

type Grafana interface {
	Enabled() bool
}

type EnvironmentHandler struct {
	renderer    *web.Renderer
	views       web.Views
	services    Services
	configs     Configs
	deployments Deployments
	releases    Releases
	grafana     Grafana
}

func NewEnvironmentHandler(renderer *web.Renderer, views web.Views, services Services, configs Configs, deployments Deployments, releases Releases, grafana Grafana) EnvironmentHandler {
	return EnvironmentHandler{
		renderer: renderer, views: views, services: services, configs: configs,
		deployments: deployments, releases: releases, grafana: grafana,
	}
}

func (h EnvironmentHandler) Show(c *gin.Context) {
	d, err := h.build(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	title := d.Service.Name + " · " + d.Env.Name
	h.renderer.Render(c, http.StatusOK, "environment", web.Page{Title: title, Nav: "services", Data: d})
}

func (h EnvironmentHandler) build(ctx context.Context, a actor.Actor, serviceSlug, envSlug string) (data, error) {
	d, err := h.frame(ctx, serviceSlug, envSlug)
	if err != nil {
		return data{}, err
	}
	if d.Revision, err = h.configs.Current(ctx, serviceSlug, envSlug); err != nil {
		return data{}, err
	}
	d.Variables = variables(d.Revision.Env)
	if d.Secrets, err = h.secretRows(ctx, serviceSlug, envSlug); err != nil {
		return data{}, err
	}
	if d.History, err = h.history(ctx, serviceSlug, envSlug); err != nil {
		return data{}, err
	}
	d.CanEdit = h.views.Can(ctx, a, authz.EditConfig, authz.Resource{TeamID: d.Service.OwnerTeamID, Environment: envSlug})
	d.Deploy, err = h.deployView(ctx, a, d)
	d.Grafana = GrafanaView{Enabled: h.grafana.Enabled(), UID: "platform-" + d.Service.Slug, Env: envSlug}
	return d, err
}

func (h EnvironmentHandler) frame(ctx context.Context, serviceSlug, envSlug string) (data, error) {
	service, err := h.services.Get(ctx, serviceSlug)
	if err != nil {
		return data{}, err
	}
	teams, err := h.views.TeamsByID(ctx, []string{service.OwnerTeamID})
	if err != nil {
		return data{}, err
	}
	envs, err := h.views.Environments(ctx)
	d := data{Service: service, Team: teams[service.OwnerTeamID]}
	for _, env := range envs {
		d.Envs = append(d.Envs, EnvTab{Env: env})
		if env.Slug == envSlug {
			d.Env = env
		}
	}
	if err == nil && d.Env.Slug == "" {
		err = errUnknownEnvironment
	}
	return d, err
}

func variables(env map[string]string) []Variable {
	result := make([]Variable, 0, len(env))
	for name, value := range env {
		result = append(result, Variable{Name: name, Value: value})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func RegisterEnvironmentRoutes(routes httpserver.Web, h EnvironmentHandler) {
	base := "/services/:service/env/:env"
	routes.GET(base, h.Show)
	routes.POST(base+"/spec", h.SaveSpec)
	routes.POST(base+"/variables", h.SetVariable)
	routes.POST(base+"/variables/:name/remove", h.RemoveVariable)
	routes.POST(base+"/secrets", h.SetSecret)
	routes.POST(base+"/secrets/:name/remove", h.RemoveSecret)
	routes.GET(base+"/deploy-panel", h.Panel)
	routes.POST(base+"/deployments", h.Deploy)
	routes.POST(base+"/config-applications", h.ApplyConfig)
	routes.POST(base+"/promotions", h.Promote)
	routes.POST(base+"/deployments/:id/rollback", h.Rollback)
}
