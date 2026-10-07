package members

import (
	"context"
	"net/http"
	"time"

	"github.com/adrianozp/platform/app/team/entities"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/gin-gonic/gin"
)

type Members interface {
	List(ctx context.Context, teamSlug string) ([]entities.Member, error)
	Put(ctx context.Context, a actor.Actor, teamSlug, login string, role entities.Role) (entities.Member, error)
	Remove(ctx context.Context, a actor.Actor, teamSlug, login string) error
}

type MembersHandler struct {
	members Members
}

func NewMembersHandler(members Members) MembersHandler {
	return MembersHandler{members: members}
}

type memberDTO struct {
	Login     string    `json:"login"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url"`
	Role      string    `json:"role"`
	Since     time.Time `json:"since"`
}

func toDTO(m entities.Member) memberDTO {
	return memberDTO{Login: m.User.Login, Name: m.User.Name, AvatarURL: m.User.AvatarURL, Role: string(m.Role), Since: m.CreatedAt}
}

type putDTO struct {
	Role string `json:"role" binding:"required"`
}

func (h MembersHandler) List(c *gin.Context) {
	list, err := h.members.List(c.Request.Context(), c.Param("team"))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]memberDTO, 0, len(list))
	for _, m := range list {
		result = append(result, toDTO(m))
	}
	c.JSON(http.StatusOK, gin.H{"members": result})
}

func (h MembersHandler) Put(c *gin.Context) {
	var body putDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	member, err := h.members.Put(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team"), c.Param("login"), entities.Role(body.Role))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, toDTO(member))
}

func (h MembersHandler) Remove(c *gin.Context) {
	if err := h.members.Remove(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team"), c.Param("login")); err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func RegisterMembersRoutes(api httpserver.API, h MembersHandler) {
	api.GET("/teams/:team/members", h.List)
	api.PUT("/teams/:team/members/:login", h.Put)
	api.DELETE("/teams/:team/members/:login", h.Remove)
}
