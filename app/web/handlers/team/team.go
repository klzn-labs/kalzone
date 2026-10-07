package team

import (
	"context"
	"net/http"

	"github.com/adrianozp/platform/app/authz"
	catalog "github.com/adrianozp/platform/app/catalog/entities"
	entities "github.com/adrianozp/platform/app/team/entities"
	"github.com/adrianozp/platform/app/web"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/gin-gonic/gin"
)

type Teams interface {
	Get(ctx context.Context, slug string) (entities.Team, error)
	Delete(ctx context.Context, a actor.Actor, slug string) error
}

type Members interface {
	List(ctx context.Context, teamSlug string) ([]entities.Member, error)
	Put(ctx context.Context, a actor.Actor, teamSlug, login string, role entities.Role) (entities.Member, error)
	Remove(ctx context.Context, a actor.Actor, teamSlug, login string) error
}

type Services interface {
	List(ctx context.Context, filter catalog.ServiceFilter) ([]catalog.Service, error)
}

type data struct {
	Team      entities.Team
	Members   []entities.Member
	Services  []web.ServiceCard
	CanManage bool
	IsAdmin   bool
}

type TeamHandler struct {
	renderer *web.Renderer
	views    web.Views
	teams    Teams
	members  Members
	services Services
}

func NewTeamHandler(renderer *web.Renderer, views web.Views, teams Teams, members Members, services Services) TeamHandler {
	return TeamHandler{renderer: renderer, views: views, teams: teams, members: members, services: services}
}

func (h TeamHandler) Show(c *gin.Context) {
	d, err := h.build(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "team", web.Page{Title: d.Team.Name, Nav: "teams", Data: d})
}

func (h TeamHandler) build(ctx context.Context, a actor.Actor, slug string) (data, error) {
	t, err := h.teams.Get(ctx, slug)
	if err != nil {
		return data{}, err
	}
	members, err := h.members.List(ctx, slug)
	if err != nil {
		return data{}, err
	}
	services, err := h.services.List(ctx, catalog.ServiceFilter{OwnerTeamIDs: []string{t.ID}})
	if err != nil {
		return data{}, err
	}
	cards, err := h.views.ServiceCards(ctx, services)
	canManage := h.views.Can(ctx, a, authz.ManageMembers, authz.Resource{TeamID: t.ID})
	return data{Team: t, Members: members, Services: cards, CanManage: canManage, IsAdmin: a.Admin}, err
}

func (h TeamHandler) PutMember(c *gin.Context) {
	login := c.PostForm("login")
	_, err := h.members.Put(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team"), login, entities.Role(c.PostForm("role")))
	h.finish(c, err, "@"+login+" is now a "+c.PostForm("role"))
}

func (h TeamHandler) RemoveMember(c *gin.Context) {
	login := c.Param("login")
	err := h.members.Remove(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team"), login)
	h.finish(c, err, "@"+login+" removed from the team")
}

func (h TeamHandler) Delete(c *gin.Context) {
	slug := c.Param("team")
	if err := h.teams.Delete(c.Request.Context(), requestctx.ActorFrom(c), slug); err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, "/teams", "Team "+slug+" deleted")
}

func (h TeamHandler) finish(c *gin.Context, err error, message string) {
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, "/teams/"+c.Param("team"), message)
}

func RegisterTeamRoutes(routes httpserver.Web, h TeamHandler) {
	routes.GET("/teams/:team", h.Show)
	routes.POST("/teams/:team/members", h.PutMember)
	routes.POST("/teams/:team/members/:login/remove", h.RemoveMember)
	routes.POST("/teams/:team/delete", h.Delete)
}
