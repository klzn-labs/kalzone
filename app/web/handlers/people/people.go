package people

import (
	"context"
	"net/http"

	identity "github.com/adrianozp/platform/app/identity/entities"
	"github.com/adrianozp/platform/app/web"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/gin-gonic/gin"
)

type Users interface {
	List(ctx context.Context) ([]identity.User, error)
	SetAdmin(ctx context.Context, a actor.Actor, login string, admin bool) (identity.User, error)
}

type Person struct {
	User  identity.User
	Teams []web.MyTeam
}

type data struct {
	People  []Person
	IsAdmin bool
	Self    string
}

type PeopleHandler struct {
	renderer *web.Renderer
	views    web.Views
	users    Users
}

func NewPeopleHandler(renderer *web.Renderer, views web.Views, users Users) PeopleHandler {
	return PeopleHandler{renderer: renderer, views: views, users: users}
}

func (h PeopleHandler) Show(c *gin.Context) {
	a := requestctx.ActorFrom(c)
	people, err := h.people(c.Request.Context())
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	d := data{People: people, IsAdmin: a.Admin, Self: a.Login}
	h.renderer.Render(c, http.StatusOK, "people", web.Page{Title: "People", Nav: "people", Data: d})
}

func (h PeopleHandler) people(ctx context.Context) ([]Person, error) {
	users, err := h.users.List(ctx)
	if err != nil {
		return nil, err
	}
	people := make([]Person, 0, len(users))
	for _, u := range users {
		teams, err := h.views.MyTeams(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		people = append(people, Person{User: u, Teams: teams})
	}
	return people, nil
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

func RegisterPeopleRoutes(routes httpserver.Web, h PeopleHandler) {
	routes.GET("/people", h.Show)
	routes.POST("/people/:login/admin", h.SetAdmin)
}
