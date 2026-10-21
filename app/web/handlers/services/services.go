package services

import (
	"context"
	"net/http"

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

const filterPlatform = "platform"

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

type listData struct {
	Filter   string
	Team     string
	Teams    []teamTab
	Cards    []web.ServiceCard
	Platform *deploy.PlatformComponent
	ShowCard bool
	Total    int
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
	platform Platform
}

func NewServicesHandler(renderer *web.Renderer, views web.Views, services Services, teams Teams, platform Platform) ServicesHandler {
	return ServicesHandler{renderer: renderer, views: views, services: services, teams: teams, platform: platform}
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
	cards, err := h.cards(ctx, a, filter, teamSlug, all, teams)
	if err != nil {
		return listData{}, err
	}
	platform, err := h.controlPlane(ctx)
	d := listData{Filter: filter, Team: teamSlug, Teams: teamTabs(teams, all), Cards: cards, Platform: platform, Total: len(all)}
	d.ShowCard = platform != nil && filter != "mine" && teamSlug == ""
	if platform != nil {
		d.Total++
	}
	return d, err
}

func (h ServicesHandler) cards(ctx context.Context, a actor.Actor, filter, teamSlug string, all []catalog.Service, teams []team.Team) ([]web.ServiceCard, error) {
	shown, err := h.filtered(ctx, a, filter, all)
	if err != nil {
		return nil, err
	}
	return h.views.ServiceCards(ctx, ownedBy(shown, teams, teamSlug))
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

func (h ServicesHandler) filtered(ctx context.Context, a actor.Actor, filter string, all []catalog.Service) ([]catalog.Service, error) {
	if filter == filterPlatform {
		return []catalog.Service{}, nil
	}
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
