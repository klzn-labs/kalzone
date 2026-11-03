package team

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/authz"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	identity "github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/usecases/accounts"
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

type Invites interface {
	Create(ctx context.Context, a actor.Actor, teamSlug, role string) (accounts.Invite, error)
	List(ctx context.Context, a actor.Actor, teamSlug string) ([]identity.AccountLink, error)
	Revoke(ctx context.Context, a actor.Actor, teamSlug, id string) error
}

type data struct {
	Team       entities.Team
	Members    []entities.Member
	Services   []web.ServiceCard
	People     []identity.User
	CanManage  bool
	IsAdmin    bool
	Passwords  bool
	CanInvite  bool
	Invites    []identity.AccountLink
	InviteURL  string
	InviteRole string
}

type TeamHandler struct {
	renderer  *web.Renderer
	views     web.Views
	teams     Teams
	members   Members
	services  Services
	users     Users
	invites   Invites
	passwords bool
}

func NewTeamHandler(renderer *web.Renderer, views web.Views, teams Teams, members Members, services Services, users Users, invites Invites, mode accounts.Mode) TeamHandler {
	return TeamHandler{
		renderer: renderer, views: views, teams: teams, members: members, services: services, users: users,
		invites: invites, passwords: !mode.GitHub,
	}
}

func (h TeamHandler) Show(c *gin.Context) {
	h.show(c, func(*data) {})
}

func (h TeamHandler) show(c *gin.Context, decorate func(*data)) {
	d, err := h.build(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	decorate(&d)
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
	cards, err := h.serviceCards(ctx, t.ID)
	if err != nil {
		return data{}, err
	}
	canManage := h.views.Can(ctx, a, authz.ManageMembers, authz.Resource{TeamID: t.ID})
	people, err := h.people(ctx, a, canManage)
	if err != nil {
		return data{}, err
	}
	d := data{Team: t, Members: members, Services: cards, People: people, CanManage: canManage, IsAdmin: a.Admin, Passwords: h.passwords}
	return h.withInvites(ctx, a, d)
}

func (h TeamHandler) serviceCards(ctx context.Context, teamID string) ([]web.ServiceCard, error) {
	services, err := h.services.List(ctx, catalog.ServiceFilter{OwnerTeamIDs: []string{teamID}})
	if err != nil {
		return nil, err
	}
	return h.views.ServiceCards(ctx, services)
}

func (h TeamHandler) withInvites(ctx context.Context, a actor.Actor, d data) (data, error) {
	if !h.passwords || !h.views.Can(ctx, a, authz.ManageInvites, authz.Resource{TeamID: d.Team.ID}) {
		return d, nil
	}
	invites, err := h.invites.List(ctx, a, d.Team.Slug)
	d.CanInvite, d.Invites = true, invites
	return d, err
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

func (h TeamHandler) CreateInvite(c *gin.Context) {
	invite, err := h.invites.Create(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team"), c.PostForm("role"))
	if err != nil {
		web.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	h.show(c, func(d *data) { d.InviteURL, d.InviteRole = invite.URL, invite.Link.Role })
}

func (h TeamHandler) RevokeInvite(c *gin.Context) {
	err := h.invites.Revoke(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team"), c.Param("id"))
	h.finish(c, err, "Invite revoked")
}

func RegisterTeamRoutes(routes httpserver.Web, h TeamHandler) {
	routes.POST("/teams/:team/invites", h.CreateInvite)
	routes.POST("/teams/:team/invites/:id/revoke", h.RevokeInvite)
	routes.GET("/teams/:team", h.Show)
	routes.POST("/teams/:team/members", h.PutMember)
	routes.POST("/teams/:team/members/:login/remove", h.RemoveMember)
	routes.POST("/teams/:team/delete", h.Delete)
}
