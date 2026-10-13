package users

import (
	"context"
	"net/http"
	"time"

	"github.com/adrianozp/platform/app/identity/entities"
	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/scope"
	"github.com/gin-gonic/gin"
)

type Users interface {
	List(ctx context.Context) ([]entities.User, error)
	FindByLogin(ctx context.Context, login string) (entities.User, error)
	SetAdmin(ctx context.Context, a actor.Actor, login string, admin bool) (entities.User, error)
}

type UsersHandler struct {
	users Users
}

func NewUsersHandler(users Users) UsersHandler {
	return UsersHandler{users: users}
}

type userDTO struct {
	ID          string    `json:"id"`
	Login       string    `json:"login"`
	Name        string    `json:"name"`
	AvatarURL   string    `json:"avatar_url"`
	Admin       bool      `json:"admin"`
	LastLoginAt time.Time `json:"last_login_at"`
}

func toDTO(u entities.User) userDTO {
	return userDTO{ID: u.ID, Login: u.Login, Name: u.Name, AvatarURL: u.AvatarURL, Admin: u.Admin, LastLoginAt: u.LastLoginAt}
}

type userParams struct {
	Login string `path:"login"`
}

type adminDTO struct {
	Admin *bool `json:"admin" binding:"required"`
}

func (h UsersHandler) List(c *gin.Context) {
	apispec.RespondWindow(c, func() ([]entities.User, error) { return h.users.List(c.Request.Context()) }, userKey, toDTO)
}

func userKey(u entities.User) string {
	return u.Login
}

func (h UsersHandler) Get(c *gin.Context) {
	user, err := h.users.FindByLogin(c.Request.Context(), c.Param("login"))
	apispec.Respond(c, http.StatusOK, toDTO(user), err)
}

func (h UsersHandler) SetAdmin(c *gin.Context) {
	var body adminDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	user, err := h.users.SetAdmin(c.Request.Context(), requestctx.ActorFrom(c), c.Param("login"), *body.Admin)
	apispec.Respond(c, http.StatusOK, toDTO(user), err)
}

func RegisterUsersRoutes(api httpserver.API, h UsersHandler) {
	api.Handle(
		apispec.Operation{ID: "listUsers", Method: http.MethodGet, Path: "/users", Summary: "List everyone who signed in, by login",
			Scope: scope.Read, Params: apispec.ListParams{}, Status: http.StatusOK, Response: apispec.Page[userDTO]{}, Handler: h.List},
		apispec.Operation{ID: "getUser", Method: http.MethodGet, Path: "/users/:login", Summary: "Show one person",
			Scope: scope.Read, Params: userParams{}, Status: http.StatusOK, Response: userDTO{}, Handler: h.Get},
		apispec.Operation{ID: "setAdmin", Method: http.MethodPut, Path: "/users/:login/admin", Summary: "Make a person a platform admin or not",
			Session: true, Params: userParams{}, Body: adminDTO{}, Status: http.StatusOK, Response: userDTO{}, Handler: h.SetAdmin},
	)
}
