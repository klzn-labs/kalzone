package overview

import (
	"context"
	"net/http"

	audit "github.com/adrianozp/platform/app/audit/entities"
	catalog "github.com/adrianozp/platform/app/catalog/entities"
	team "github.com/adrianozp/platform/app/team/entities"
	"github.com/adrianozp/platform/app/web"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/gin-gonic/gin"
)

type Services interface {
	List(ctx context.Context, filter catalog.ServiceFilter) ([]catalog.Service, error)
}

type Teams interface {
	List(ctx context.Context) ([]team.Team, error)
}

type Events interface {
	List(ctx context.Context, filter audit.Filter) ([]audit.Event, error)
}

type TeamSection struct {
	web.MyTeam
	Services []web.ServiceCard
}

type data struct {
	TotalServices int
	TotalTeams    int
	MyServices    int
	MyTeams       []TeamSection
	Activity      []web.Activity
}

type OverviewHandler struct {
	renderer *web.Renderer
	views    web.Views
	services Services
	teams    Teams
	events   Events
}

func NewOverviewHandler(renderer *web.Renderer, views web.Views, services Services, teams Teams, events Events) OverviewHandler {
	return OverviewHandler{renderer: renderer, views: views, services: services, teams: teams, events: events}
}

func (h OverviewHandler) Show(c *gin.Context) {
	d, err := h.build(c.Request.Context(), requestctx.ActorFrom(c).UserID)
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "overview", web.Page{Title: "Overview", Nav: "overview", Data: d})
}

func (h OverviewHandler) build(ctx context.Context, userID string) (data, error) {
	all, err := h.services.List(ctx, catalog.ServiceFilter{})
	if err != nil {
		return data{}, err
	}
	teams, err := h.teams.List(ctx)
	if err != nil {
		return data{}, err
	}
	sections, err := h.sections(ctx, userID)
	if err != nil {
		return data{}, err
	}
	events, err := h.events.List(ctx, audit.Filter{Limit: 12})
	d := data{TotalServices: len(all), TotalTeams: len(teams), MyTeams: sections, Activity: web.ActivityFrom(events)}
	for _, s := range sections {
		d.MyServices += len(s.Services)
	}
	return d, err
}

func (h OverviewHandler) sections(ctx context.Context, userID string) ([]TeamSection, error) {
	mine, err := h.views.MyTeams(ctx, userID)
	if err != nil {
		return nil, err
	}
	sections := make([]TeamSection, 0, len(mine))
	for _, t := range mine {
		services, err := h.services.List(ctx, catalog.ServiceFilter{OwnerTeamIDs: []string{t.Team.ID}})
		if err != nil {
			return nil, err
		}
		cards, err := h.views.ServiceCards(ctx, services)
		if err != nil {
			return nil, err
		}
		sections = append(sections, TeamSection{MyTeam: t, Services: cards})
	}
	return sections, nil
}

func RegisterOverviewRoutes(routes httpserver.Web, h OverviewHandler) {
	routes.GET("/", h.Show)
}
