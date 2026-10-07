package services

import (
	"context"
	"net/http"

	catalog "github.com/adrianozp/platform/app/catalog/entities"
	team "github.com/adrianozp/platform/app/team/entities"
	"github.com/adrianozp/platform/app/web"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/gin-gonic/gin"
)

type Services interface {
	List(ctx context.Context, filter catalog.ServiceFilter) ([]catalog.Service, error)
	Create(ctx context.Context, a actor.Actor, n catalog.NewService) (catalog.Service, error)
}

type Teams interface {
	List(ctx context.Context) ([]team.Team, error)
}

type listData struct {
	Filter string
	Cards  []web.ServiceCard
	Total  int
}

type newData struct {
	Teams    []team.Team
	Selected string
	CanPick  bool
}

type ServicesHandler struct {
	renderer *web.Renderer
	views    web.Views
	services Services
	teams    Teams
}

func NewServicesHandler(renderer *web.Renderer, views web.Views, services Services, teams Teams) ServicesHandler {
	return ServicesHandler{renderer: renderer, views: views, services: services, teams: teams}
}

func (h ServicesHandler) List(c *gin.Context) {
	d, err := h.list(c.Request.Context(), requestctx.ActorFrom(c), c.Query("filter"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "services", web.Page{Title: "Services", Nav: "services", Data: d})
}

func (h ServicesHandler) list(ctx context.Context, a actor.Actor, filter string) (listData, error) {
	all, err := h.services.List(ctx, catalog.ServiceFilter{})
	if err != nil {
		return listData{}, err
	}
	shown, err := h.filtered(ctx, a, filter, all)
	if err != nil {
		return listData{}, err
	}
	cards, err := h.views.ServiceCards(ctx, shown)
	return listData{Filter: filter, Cards: cards, Total: len(all)}, err
}

func (h ServicesHandler) filtered(ctx context.Context, a actor.Actor, filter string, all []catalog.Service) ([]catalog.Service, error) {
	if filter != "mine" {
		return all, nil
	}
	mine, err := h.views.MyTeams(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	return h.services.List(ctx, catalog.ServiceFilter{OwnerTeamIDs: web.TeamIDs(mine)})
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
