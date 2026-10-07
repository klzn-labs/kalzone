package users

import (
	"context"
	"net/http"
	"time"

	"github.com/adrianozp/platform/app/identity/entities"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
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

func (h UsersHandler) List(c *gin.Context) {
	list, err := h.users.List(c.Request.Context())
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]userDTO, 0, len(list))
	for _, u := range list {
		result = append(result, toDTO(u))
	}
	c.JSON(http.StatusOK, gin.H{"users": result})
}

func (h UsersHandler) Get(c *gin.Context) {
	user, err := h.users.FindByLogin(c.Request.Context(), c.Param("login"))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, toDTO(user))
}

type adminDTO struct {
	Admin *bool `json:"admin" binding:"required"`
}

func (h UsersHandler) SetAdmin(c *gin.Context) {
	var body adminDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	user, err := h.users.SetAdmin(c.Request.Context(), requestctx.ActorFrom(c), c.Param("login"), *body.Admin)
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, toDTO(user))
}

func RegisterUsersRoutes(api httpserver.API, h UsersHandler) {
	api.GET("/users", h.List)
	api.GET("/users/:login", h.Get)
	api.PUT("/users/:login/admin", h.SetAdmin)
}
