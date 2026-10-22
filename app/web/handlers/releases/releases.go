package releases

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/authz"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	identity "github.com/klzn-labs/kalzone/app/identity/entities"
	release "github.com/klzn-labs/kalzone/app/release/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Services interface {
	GetFor(ctx context.Context, a actor.Actor, slug string) (catalog.Service, error)
	Update(ctx context.Context, a actor.Actor, slug string, c catalog.ServiceChanges) (catalog.Service, error)
}

type Releases interface {
	List(ctx context.Context, serviceSlug string, limit int) ([]release.Release, error)
	Request(ctx context.Context, a actor.Actor, serviceSlug, ref string) (release.Release, error)
	Register(ctx context.Context, a actor.Actor, serviceSlug, tag string) (release.Release, error)
	Resolve(ctx context.Context, serviceSlug, ref string) (release.Release, error)
}

type Changelogs interface {
	Get(ctx context.Context, rel release.Release) (release.Changelog, error)
	Many(ctx context.Context, releaseIDs []string) (map[string]release.Changelog, error)
}

const recentReleases = 5

type catalogEnv = catalog.Environment

type Row struct {
	Release         release.Release
	RequestedBy     identity.User
	Changes         string
	FailureSummary  string
	MissingWorkflow bool
}

type data struct {
	Service  catalog.Service
	Team     team.Team
	Envs     []EnvTab
	Rows     []Row
	CanEdit  bool
	CanBuild bool
	Building bool
	Wiring   Wiring
	Snippet  string
	Uses     string
}

func (d data) RecentRows() []Row {
	return d.Rows[:min(len(d.Rows), recentReleases)]
}

func (d data) OlderRows() []Row {
	return d.Rows[min(len(d.Rows), recentReleases):]
}

type EnvTab struct {
	Env catalog.Environment
}

type ReleasesHandler struct {
	renderer   *web.Renderer
	views      web.Views
	services   Services
	releases   Releases
	changelogs Changelogs
}

func NewReleasesHandler(renderer *web.Renderer, views web.Views, services Services, releases Releases, changelogs Changelogs) ReleasesHandler {
	return ReleasesHandler{renderer: renderer, views: views, services: services, releases: releases, changelogs: changelogs}
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
	d, err := h.frame(ctx, a, slug)
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
	d.Wiring = wiringOf(d.Service, list)
	return d, nil
}

func (h ReleasesHandler) frame(ctx context.Context, a actor.Actor, slug string) (data, error) {
	service, err := h.services.GetFor(ctx, a, slug)
	if err != nil {
		return data{}, err
	}
	teams, err := h.views.TeamsByID(ctx, []string{service.OwnerTeamID})
	if err != nil {
		return data{}, err
	}
	envs, err := h.views.Environments(ctx)
	d := data{Service: service, Team: teams[service.OwnerTeamID], Snippet: callerWorkflow, Uses: buildWorkflowUses}
	for _, env := range envs {
		d.Envs = append(d.Envs, EnvTab{Env: env})
	}
	return d, err
}

func (h ReleasesHandler) rows(ctx context.Context, list []release.Release) ([]Row, error) {
	userIDs, releaseIDs, numbers := make([]string, 0, len(list)), make([]string, 0, len(list)), map[string]int{}
	for _, r := range list {
		userIDs, releaseIDs, numbers[r.ID] = append(userIDs, r.RequestedBy), append(releaseIDs, r.ID), r.Number
	}
	users, err := h.views.UsersByID(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	logs, err := h.changelogs.Many(ctx, releaseIDs)
	rows := make([]Row, 0, len(list))
	for _, r := range list {
		rows = append(rows, rowOf(r, users[r.RequestedBy], changesLabel(logs[r.ID], numbers)))
	}
	return rows, err
}

func rowOf(r release.Release, by identity.User, changes string) Row {
	return Row{Release: r, RequestedBy: by, Changes: changes, FailureSummary: failureSummary(r.Failure), MissingWorkflow: missingWorkflow(r.Failure)}
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
	autoBuild, autoDeploy := c.PostForm("auto_build") == "on", c.PostForm("auto_deploy_dev") == "on"
	workflow := strings.TrimSpace(c.PostForm("build_workflow"))
	changes := catalog.ServiceChanges{AutoBuild: &autoBuild, AutoDeployDev: &autoDeploy, BuildWorkflow: &workflow}
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
	routes.GET("/services/:service/releases/:number", h.Detail)
	routes.POST("/services/:service/releases", h.Build)
	routes.POST("/services/:service/releases/registrations", h.Register)
	routes.POST("/services/:service/build-settings", h.Settings)
}
