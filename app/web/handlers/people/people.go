package people

import (
	"context"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	identity "github.com/klzn-labs/kalzone/app/identity/entities"
	modes "github.com/klzn-labs/kalzone/app/identity/usecases/accounts"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
)

type Users interface {
	ListFor(ctx context.Context, a actor.Actor) ([]identity.User, error)
	SetAdmin(ctx context.Context, a actor.Actor, login string, admin bool) (identity.User, error)
}

type People interface {
	Remove(ctx context.Context, a actor.Actor, login string) error
	IssueReset(ctx context.Context, a actor.Actor, login string) (string, error)
	ChangePassword(ctx context.Context, a actor.Actor, current, next, confirm, keepSessionHash string) error
}

type Person struct {
	User  identity.User
	Teams []web.MyTeam
}

type data struct {
	People    []Person
	IsAdmin   bool
	Self      string
	Passwords bool
	ResetFor  string
	ResetURL  string
}

type PeopleHandler struct {
	renderer  *web.Renderer
	views     web.Views
	users     Users
	accounts  People
	passwords bool
}

func NewPeopleHandler(renderer *web.Renderer, views web.Views, users Users, accounts People, mode modes.Mode) PeopleHandler {
	return PeopleHandler{renderer: renderer, views: views, users: users, accounts: accounts, passwords: !mode.GitHub}
}

func (h PeopleHandler) Show(c *gin.Context) {
	h.show(c, data{})
}

func (h PeopleHandler) show(c *gin.Context, d data) {
	a := requestctx.ActorFrom(c)
	if a.Guest {
		web.ErrorPage(c, h.renderer, web.ErrNotInDemo)
		return
	}
	people, err := h.people(c.Request.Context(), a)
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	d.People, d.IsAdmin, d.Self, d.Passwords = people, a.Admin, a.Login, h.passwords
	h.renderer.Render(c, http.StatusOK, "people", web.Page{Title: "People", Nav: "people", Data: d})
}

func (h PeopleHandler) people(ctx context.Context, a actor.Actor) ([]Person, error) {
	users, err := h.users.ListFor(ctx, a)
	if err != nil {
		return nil, err
	}
	people := make([]Person, 0, len(users))
	for _, u := range users {
		teams, err := h.views.MyTeams(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		people = append(people, Person{User: u, Teams: visible(a, teams)})
	}
	return people, nil
}

func visible(a actor.Actor, teams []web.MyTeam) []web.MyTeam {
	return slices.DeleteFunc(teams, func(t web.MyTeam) bool { return !a.Sees(t.Team.ID) })
}

func (h PeopleHandler) SetAdmin(c *gin.Context) {
	admin := c.PostForm("admin") == "true"
	user, err := h.users.SetAdmin(c.Request.Context(), requestctx.ActorFrom(c), c.Param("login"), admin)
	if err != nil {
		web.Fail(c, err)
		return
	}
	message := "@" + user.Login + " is no longer an admin"
	if admin {
		message = "@" + user.Login + " is now an admin"
	}
	web.Done(c, "/people", message)
}

func (h PeopleHandler) Remove(c *gin.Context) {
	login := c.Param("login")
	if err := h.accounts.Remove(c.Request.Context(), requestctx.ActorFrom(c), login); err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, "/people", "@"+login+" was removed")
}

func (h PeopleHandler) IssueReset(c *gin.Context) {
	login := c.Param("login")
	url, err := h.accounts.IssueReset(c.Request.Context(), requestctx.ActorFrom(c), login)
	if err != nil {
		web.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	h.show(c, data{ResetFor: login, ResetURL: url})
}

func RegisterPeopleRoutes(routes httpserver.Web, h PeopleHandler) {
	routes.GET("/people", h.Show)
	routes.POST("/people/:login/admin", h.SetAdmin)
	routes.POST("/people/:login/remove", h.Remove)
	routes.POST("/people/:login/reset", h.IssueReset)
	routes.GET("/account/password", h.ShowPassword)
	routes.POST("/account/password", h.ChangePassword)
}
