package members

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/scope"
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
	Role      string    `json:"role" enum:"member,maintainer"`
	Since     time.Time `json:"since"`
}

func toDTO(m entities.Member) memberDTO {
	return memberDTO{Login: m.User.Login, Name: m.User.Name, AvatarURL: m.User.AvatarURL, Role: string(m.Role), Since: m.CreatedAt}
}

type putDTO struct {
	Role string `json:"role" binding:"required" enum:"member,maintainer"`
}

type memberParams struct {
	Team  string `path:"team"`
	Login string `path:"login"`
}

type listParams struct {
	Team string `path:"team"`
	apispec.ListParams
}

func (h MembersHandler) List(c *gin.Context) {
	load := func() ([]entities.Member, error) { return h.members.List(c.Request.Context(), c.Param("team")) }
	apispec.RespondWindow(c, load, memberKey, toDTO)
}

func memberKey(m entities.Member) string {
	return m.User.Login
}

func (h MembersHandler) Put(c *gin.Context) {
	var body putDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	member, err := h.members.Put(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team"), c.Param("login"), entities.Role(body.Role))
	apispec.Respond(c, http.StatusOK, toDTO(member), err)
}

func (h MembersHandler) Remove(c *gin.Context) {
	if err := h.members.Remove(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team"), c.Param("login")); err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func RegisterMembersRoutes(api httpserver.API, h MembersHandler) {
	api.Handle(
		apispec.Operation{ID: "listMembers", Method: http.MethodGet, Path: "/teams/:team/members", Summary: "List a team's members in the order they joined",
			Scope: scope.Read, Params: listParams{}, Status: http.StatusOK, Response: apispec.Page[memberDTO]{}, Handler: h.List},
		apispec.Operation{ID: "putMember", Method: http.MethodPut, Path: "/teams/:team/members/:login", Summary: "Add a member or change their role",
			Scope: scope.TeamsAdmin, Params: memberParams{}, Body: putDTO{}, Status: http.StatusOK, Response: memberDTO{}, Handler: h.Put},
		apispec.Operation{ID: "removeMember", Method: http.MethodDelete, Path: "/teams/:team/members/:login", Summary: "Remove a member",
			Scope: scope.TeamsAdmin, Params: memberParams{}, Status: http.StatusNoContent, Handler: h.Remove},
	)
}
