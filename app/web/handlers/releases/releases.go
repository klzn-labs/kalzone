package releases

import (
	"context"
	"net/http"
	"strings"

	"github.com/adrianozp/platform/app/authz"
	catalog "github.com/adrianozp/platform/app/catalog/entities"
	identity "github.com/adrianozp/platform/app/identity/entities"
	release "github.com/adrianozp/platform/app/release/entities"
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
}

type Releases interface {
	List(ctx context.Context, serviceSlug string, limit int) ([]release.Release, error)
	Request(ctx context.Context, a actor.Actor, serviceSlug, ref string) (release.Release, error)
	Register(ctx context.Context, a actor.Actor, serviceSlug, tag string) (release.Release, error)
}

type Row struct {
	Release     release.Release
	RequestedBy identity.User
}

type data struct {
	Service  catalog.Service
	Team     team.Team
	Envs     []EnvTab
	Rows     []Row
	CanEdit  bool
	CanBuild bool
	Building bool
	Snippet  string
}

type EnvTab struct {
	Env catalog.Environment
}

type ReleasesHandler struct {
	renderer *web.Renderer
	views    web.Views
	services Services
	releases Releases
}

func NewReleasesHandler(renderer *web.Renderer, views web.Views, services Services, releases Releases) ReleasesHandler {
	return ReleasesHandler{renderer: renderer, views: views, services: services, releases: releases}
}

func (h ReleasesHandler) Show(c *gin.Context) {
	d, err := h.build(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "releases", web.Page{Title: d.Service.Name + " · Releases", Nav: "services", Data: d})
}

func (h ReleasesHandler) build(ctx context.Context, a actor.Actor, slug string) (data, error) {
	d, err := h.frame(ctx, slug)
	if err != nil {
		return data{}, err
	}
	list, err := h.releases.List(ctx, slug, 50)
	if err != nil {
		return data{}, err
	}
	if d.Rows, err = h.rows(ctx, list); err != nil {
		return data{}, err
	}
	resource := authz.Resource{TeamID: d.Service.OwnerTeamID}
	d.CanEdit = h.views.Can(ctx, a, authz.EditService, resource)
	d.CanBuild = h.views.Can(ctx, a, authz.CreateRelease, resource)
	d.Building = anyBuilding(list)
	return d, nil
}

func (h ReleasesHandler) frame(ctx context.Context, slug string) (data, error) {
	service, err := h.services.Get(ctx, slug)
	if err != nil {
		return data{}, err
	}
	teams, err := h.views.TeamsByID(ctx, []string{service.OwnerTeamID})
	if err != nil {
		return data{}, err
	}
	envs, err := h.views.Environments(ctx)
	d := data{Service: service, Team: teams[service.OwnerTeamID], Snippet: callerWorkflow}
	for _, env := range envs {
		d.Envs = append(d.Envs, EnvTab{Env: env})
	}
	return d, err
}

func (h ReleasesHandler) rows(ctx context.Context, list []release.Release) ([]Row, error) {
	ids := make([]string, 0, len(list))
	for _, r := range list {
		ids = append(ids, r.RequestedBy)
	}
	users, err := h.views.UsersByID(ctx, ids)
	rows := make([]Row, 0, len(list))
	for _, r := range list {
		rows = append(rows, Row{Release: r, RequestedBy: users[r.RequestedBy]})
	}
	return rows, err
}

func anyBuilding(list []release.Release) bool {
	for _, r := range list {
		if !r.Final() {
			return true
		}
	}
	return false
}

func (h ReleasesHandler) Build(c *gin.Context) {
	rel, err := h.releases.Request(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.PostForm("ref"))
	h.finish(c, err, "Build r"+itoa(rel.Number)+" started")
}

func (h ReleasesHandler) Register(c *gin.Context) {
	rel, err := h.releases.Register(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.PostForm("tag"))
	h.finish(c, err, "Release r"+itoa(rel.Number)+" registered")
}

func (h ReleasesHandler) Settings(c *gin.Context) {
	autoBuild := c.PostForm("auto_build") == "on"
	workflow := strings.TrimSpace(c.PostForm("build_workflow"))
	changes := catalog.ServiceChanges{AutoBuild: &autoBuild, BuildWorkflow: &workflow}
	_, err := h.services.Update(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), changes)
	h.finish(c, err, "Build settings saved")
}

func (h ReleasesHandler) finish(c *gin.Context, err error, message string) {
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, "/services/"+c.Param("service")+"/releases", message)
}

func RegisterReleasesRoutes(routes httpserver.Web, h ReleasesHandler) {
	routes.GET("/services/:service/releases", h.Show)
	routes.POST("/services/:service/releases", h.Build)
	routes.POST("/services/:service/releases/registrations", h.Register)
	routes.POST("/services/:service/build-settings", h.Settings)
}
