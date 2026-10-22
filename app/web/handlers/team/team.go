package team

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/authz"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	identity "github.com/klzn-labs/kalzone/app/identity/entities"
	entities "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Teams interface {
	GetFor(ctx context.Context, a actor.Actor, slug string) (entities.Team, error)
	Delete(ctx context.Context, a actor.Actor, slug string) error
}

type Members interface {
	List(ctx context.Context, a actor.Actor, teamSlug string) ([]entities.Member, error)
	Put(ctx context.Context, a actor.Actor, teamSlug, login string, role entities.Role) (entities.Member, error)
	Remove(ctx context.Context, a actor.Actor, teamSlug, login string) error
}

type Services interface {
	List(ctx context.Context, filter catalog.ServiceFilter) ([]catalog.Service, error)
}

type Users interface {
	ListFor(ctx context.Context, a actor.Actor) ([]identity.User, error)
}

type data struct {
	Team      entities.Team
	Members   []entities.Member
	Services  []web.ServiceCard
	People    []identity.User
	CanManage bool
	IsAdmin   bool
}

type TeamHandler struct {
	renderer *web.Renderer
	views    web.Views
	teams    Teams
	members  Members
	services Services
	users    Users
}

func NewTeamHandler(renderer *web.Renderer, views web.Views, teams Teams, members Members, services Services, users Users) TeamHandler {
	return TeamHandler{renderer: renderer, views: views, teams: teams, members: members, services: services, users: users}
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
	t, err := h.teams.GetFor(ctx, a, slug)
	if err != nil {
		return data{}, err
	}
	members, err := h.members.List(ctx, a, slug)
	if err != nil {
		return data{}, err
	}
	services, err := h.services.List(ctx, catalog.ServiceFilter{OwnerTeamIDs: []string{t.ID}})
	if err != nil {
		return data{}, err
	}
	cards, err := h.views.ServiceCards(ctx, services)
	if err != nil {
		return data{}, err
	}
	canManage := h.views.Can(ctx, a, authz.ManageMembers, authz.Resource{TeamID: t.ID})
	people, err := h.people(ctx, a, canManage)
	return data{Team: t, Members: members, Services: cards, People: people, CanManage: canManage, IsAdmin: a.Admin}, err
}

func (h TeamHandler) people(ctx context.Context, a actor.Actor, canManage bool) ([]identity.User, error) {
	if !canManage {
		return nil, nil
	}
	return h.users.ListFor(ctx, a)
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
