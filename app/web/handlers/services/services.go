package services

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
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
	Team   string
	Teams  []teamTab
	Cards  []web.ServiceCard
	Total  int
}

type teamTab struct {
	Slug  string
	Name  string
	Count int
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
	d, err := h.list(c.Request.Context(), requestctx.ActorFrom(c), c.Query("filter"), c.Query("team"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "services", web.Page{Title: "Services", Nav: "services", Data: d})
}

func (h ServicesHandler) list(ctx context.Context, a actor.Actor, filter, teamSlug string) (listData, error) {
	all, err := h.services.List(ctx, catalog.ServiceFilter{})
	if err != nil {
		return listData{}, err
	}
	teams, err := h.teams.List(ctx)
	if err != nil {
		return listData{}, err
	}
	shown, err := h.filtered(ctx, a, filter, all)
	if err != nil {
		return listData{}, err
	}
	shown = ownedBy(shown, teams, teamSlug)
	cards, err := h.views.ServiceCards(ctx, shown)
	return listData{Filter: filter, Team: teamSlug, Teams: teamTabs(teams, all), Cards: cards, Total: len(all)}, err
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

func ownedBy(services []catalog.Service, teams []team.Team, teamSlug string) []catalog.Service {
	if teamSlug == "" {
		return services
	}
	teamID := teamIDBySlug(teams, teamSlug)
	owned := []catalog.Service{}
	for _, s := range services {
		if s.OwnerTeamID == teamID {
			owned = append(owned, s)
		}
	}
	return owned
}

func teamIDBySlug(teams []team.Team, slug string) string {
	for _, t := range teams {
		if t.Slug == slug {
			return t.ID
		}
	}
	return ""
}

func teamTabs(teams []team.Team, services []catalog.Service) []teamTab {
	counts := map[string]int{}
	for _, s := range services {
		counts[s.OwnerTeamID]++
	}
	tabs := []teamTab{}
	for _, t := range teams {
		if counts[t.ID] > 0 {
			tabs = append(tabs, teamTab{Slug: t.Slug, Name: t.Name, Count: counts[t.ID]})
		}
	}
	return tabs
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
