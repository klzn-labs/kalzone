package teams

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

type Teams interface {
	ListFor(ctx context.Context, a actor.Actor) ([]team.Team, error)
	Create(ctx context.Context, a actor.Actor, slug, name string) (team.Team, error)
}

type Members interface {
	List(ctx context.Context, a actor.Actor, teamSlug string) ([]team.Member, error)
}

type Services interface {
	List(ctx context.Context, filter catalog.ServiceFilter) ([]catalog.Service, error)
}

type TeamCard struct {
	Team     team.Team
	Members  []team.Member
	Services int
	MyRole   team.Role
}

type data struct {
	Cards   []TeamCard
	IsAdmin bool
}

type TeamsHandler struct {
	renderer *web.Renderer
	teams    Teams
	members  Members
	services Services
}

func NewTeamsHandler(renderer *web.Renderer, teams Teams, members Members, services Services) TeamsHandler {
	return TeamsHandler{renderer: renderer, teams: teams, members: members, services: services}
}

func (h TeamsHandler) List(c *gin.Context) {
	a := requestctx.ActorFrom(c)
	cards, err := h.cards(c.Request.Context(), a)
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "teams", web.Page{Title: "Teams", Nav: "teams", Data: data{Cards: cards, IsAdmin: a.Admin}})
}

func (h TeamsHandler) cards(ctx context.Context, a actor.Actor) ([]TeamCard, error) {
	teams, err := h.teams.ListFor(ctx, a)
	if err != nil {
		return nil, err
	}
	services, err := h.services.List(ctx, catalog.ServiceFilter{})
	if err != nil {
		return nil, err
	}
	cards := make([]TeamCard, 0, len(teams))
	for _, t := range teams {
		members, err := h.members.List(ctx, a, t.Slug)
		if err != nil {
			return nil, err
		}
		cards = append(cards, TeamCard{Team: t, Members: members, Services: countOwned(services, t.ID), MyRole: roleOf(members, a.UserID)})
	}
	return cards, nil
}

func countOwned(services []catalog.Service, teamID string) int {
	count := 0
	for _, s := range services {
		if s.OwnerTeamID == teamID {
			count++
		}
	}
	return count
}

func roleOf(members []team.Member, userID string) team.Role {
	for _, m := range members {
		if m.User.ID == userID {
			return m.Role
		}
	}
	return ""
}

func (h TeamsHandler) Create(c *gin.Context) {
	t, err := h.teams.Create(c.Request.Context(), requestctx.ActorFrom(c), c.PostForm("slug"), c.PostForm("name"))
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, "/teams/"+t.Slug, "Team "+t.Name+" created")
}

func RegisterTeamsRoutes(routes httpserver.Web, h TeamsHandler) {
	routes.GET("/teams", h.List)
	routes.POST("/teams", h.Create)
}
