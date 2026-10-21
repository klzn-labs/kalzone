package services

import (
	"context"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

type Services interface {
	List(ctx context.Context, filter catalog.ServiceFilter) ([]catalog.Service, error)
	Create(ctx context.Context, a actor.Actor, n catalog.NewService) (catalog.Service, error)
}

type Teams interface {
	List(ctx context.Context) ([]team.Team, error)
}

type Platform interface {
	Component(ctx context.Context, name string) (deploy.PlatformComponent, error)
}

type Section struct {
	Team   team.Team
	Rows   []web.ServiceStatus
	Broken int
}

type listData struct {
	Sections  []Section
	Platform  *deploy.PlatformComponent
	Prod      catalog.Environment
	Total     int
	Broken    int
	Deploying int
}

type newData struct {
	Teams    []team.Team
	Selected string
	CanPick  bool
}

type ServicesHandler struct {
	renderer *web.Renderer
	views    web.Views
	statuses web.Statuses
	services Services
	teams    Teams
	platform Platform
}

func NewServicesHandler(renderer *web.Renderer, views web.Views, statuses web.Statuses, services Services, teams Teams, platform Platform) ServicesHandler {
	return ServicesHandler{renderer: renderer, views: views, statuses: statuses, services: services, teams: teams, platform: platform}
}

func (h ServicesHandler) List(c *gin.Context) {
	d, err := h.list(c.Request.Context(), requestctx.ActorFrom(c))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "services", web.Page{Title: "Services", Nav: "services", Data: d})
}

func (h ServicesHandler) list(ctx context.Context, a actor.Actor) (listData, error) {
	d, services, err := h.catalog(ctx, a)
	if err != nil {
		return listData{}, err
	}
	statuses, err := h.statuses.Now(ctx, a, services, d.Prod.Slug)
	if err != nil {
		return listData{}, err
	}
	teams, err := h.teams.List(ctx)
	d.Sections, d.Total = sections(teams, statuses), len(statuses)
	d.Broken, d.Deploying = counts(statuses)
	return d, err
}

func (h ServicesHandler) catalog(ctx context.Context, a actor.Actor) (listData, []catalog.Service, error) {
	all, err := h.services.List(ctx, catalog.ServiceFilter{})
	if err != nil {
		return listData{}, nil, err
	}
	envs, err := h.views.Environments(ctx)
	if err != nil {
		return listData{}, nil, err
	}
	prod, _ := web.ProdEnvironment(envs)
	platform, err := h.controlPlane(ctx)
	return listData{Prod: prod, Platform: platform}, visible(a, all), err
}

func visible(a actor.Actor, all []catalog.Service) []catalog.Service {
	shown := []catalog.Service{}
	for _, s := range all {
		if a.Sees(s.OwnerTeamID) {
			shown = append(shown, s)
		}
	}
	return shown
}

func sections(teams []team.Team, statuses []web.ServiceStatus) []Section {
	result := []Section{}
	for _, t := range teams {
		s := Section{Team: t}
		for _, st := range statuses {
			s.add(st)
		}
		if len(s.Rows) > 0 {
			result = append(result, s.sorted())
		}
	}
	return result
}

func (s *Section) add(st web.ServiceStatus) {
	if st.Service.OwnerTeamID != s.Team.ID {
		return
	}
	s.Rows = append(s.Rows, st)
	if st.Broken() {
		s.Broken++
	}
}

func (s Section) sorted() Section {
	sort.SliceStable(s.Rows, func(i, j int) bool {
		if s.Rows[i].Broken() != s.Rows[j].Broken() {
			return s.Rows[i].Broken()
		}
		return s.Rows[i].Service.Name < s.Rows[j].Service.Name
	})
	return s
}

func counts(statuses []web.ServiceStatus) (int, int) {
	broken, deploying := 0, 0
	for _, st := range statuses {
		if st.Broken() {
			broken++
		}
		if st.Running() != nil {
			deploying++
		}
	}
	return broken, deploying
}

func (h ServicesHandler) controlPlane(ctx context.Context) (*deploy.PlatformComponent, error) {
	c, err := h.platform.Component(ctx, deploy.PlatformControlPlane)
	if apperror.CodeOf(err) == apperror.CodeNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (h ServicesHandler) New(c *gin.Context) {
	a := requestctx.ActorFrom(c)
	teams, err := h.selectableTeams(c.Request.Context(), a)
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	d := newData{Teams: teams, Selected: c.Query("team"), CanPick: len(teams) > 0}
	h.renderer.Render(c, http.StatusOK, "service_new", web.Page{Title: "New service", Nav: "services", Data: d})
}

func (h ServicesHandler) selectableTeams(ctx context.Context, a actor.Actor) ([]team.Team, error) {
	if a.Admin {
		return h.teams.List(ctx)
	}
	mine, err := h.views.MyTeams(ctx, a.UserID)
	teams := make([]team.Team, 0, len(mine))
	for _, t := range mine {
		teams = append(teams, t.Team)
	}
	return teams, err
}

func (h ServicesHandler) Create(c *gin.Context) {
	service, err := h.services.Create(c.Request.Context(), requestctx.ActorFrom(c), catalog.NewService{
		Slug: c.PostForm("slug"), Name: c.PostForm("name"), Description: c.PostForm("description"),
		OwnerTeam: c.PostForm("team"), GitHubRepo: c.PostForm("github_repo"), ImageRepository: c.PostForm("image_repository"),
	})
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, "/services/"+service.Slug, "Service "+service.Name+" created")
}

func RegisterServicesRoutes(routes httpserver.Web, h ServicesHandler) {
	routes.GET("/services", h.List)
	routes.GET("/services/new", h.New)
	routes.POST("/services", h.Create)
}
