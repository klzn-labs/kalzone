package environment

import (
	"context"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/authz"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	envconfig "github.com/klzn-labs/kalzone/app/envconfig/entities"
	identity "github.com/klzn-labs/kalzone/app/identity/entities"
	observe "github.com/klzn-labs/kalzone/app/observe/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Services interface {
	GetFor(ctx context.Context, a actor.Actor, slug string) (catalog.Service, error)
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
	Guest     bool
	DemoNote  string
}

type GrafanaView struct {
	Enabled bool
	Org     int
	UID     string
	Env     string
}

type Grafana interface {
	Enabled() bool
	OrgID(name string) (int, bool)
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
	d, err := h.frame(ctx, a, serviceSlug, envSlug)
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
	d.Grafana = h.grafanaView(d, envSlug)
	d.Guest, d.DemoNote = a.Guest, demoNote(d.Env)
	return d, err
}

func (h EnvironmentHandler) grafanaView(d data, envSlug string) GrafanaView {
	name := observe.TeamOrgName(d.Team.Slug)
	if d.Service.Simulated {
		name = observe.DemoOrg
	}
	org, known := h.grafana.OrgID(name)
	return GrafanaView{Enabled: h.grafana.Enabled() && known, Org: org, UID: observe.DashboardUID(d.Service.Slug), Env: envSlug}
}

func (h EnvironmentHandler) frame(ctx context.Context, a actor.Actor, serviceSlug, envSlug string) (data, error) {
	service, err := h.services.GetFor(ctx, a, serviceSlug)
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

func demoNote(env catalog.Environment) string {
	if env.Protected {
		return "In your own team, deploying to " + env.Name + " opens a change request that a maintainer approves."
	}
	return "In your own team, members deploy to " + env.Name + " straight away, blue-green with an automatic rollback."
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
