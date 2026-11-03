package accounts

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/identity/entities"
	usecase "github.com/klzn-labs/kalzone/app/identity/usecases/accounts"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

type Invites interface {
	Create(ctx context.Context, a actor.Actor, teamSlug, role string) (usecase.Invite, error)
	List(ctx context.Context, a actor.Actor, teamSlug string) ([]entities.AccountLink, error)
	Revoke(ctx context.Context, a actor.Actor, teamSlug, id string) error
}

type InvitesHandler struct {
	invites Invites
}

func NewInvitesHandler(invites Invites) InvitesHandler {
	return InvitesHandler{invites: invites}
}

type createInviteDTO struct {
	Role string `json:"role" binding:"required" enum:"member,maintainer"`
}

type createdInviteDTO struct {
	ID        string    `json:"id"`
	Role      string    `json:"role" enum:"member,maintainer"`
	ExpiresAt time.Time `json:"expires_at"`
	URL       string    `json:"url"`
}

type inviteDTO struct {
	ID        string    `json:"id"`
	Role      string    `json:"role" enum:"member,maintainer"`
	CreatedBy string    `json:"created_by"`
	ExpiresAt time.Time `json:"expires_at"`
}

type teamParams struct {
	Team string `path:"team"`
}

type inviteParams struct {
	Team string `path:"team"`
	ID   string `path:"id"`
}

type listInvitesParams struct {
	Team string `path:"team"`
	apispec.ListParams
}

func toInviteDTO(l entities.AccountLink) inviteDTO {
	return inviteDTO{ID: l.ID, Role: l.Role, CreatedBy: l.CreatedByLogin, ExpiresAt: l.ExpiresAt}
}

func inviteKey(l entities.AccountLink) string {
	return l.ID
}

func (h InvitesHandler) Create(c *gin.Context) {
	var body createInviteDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	invite, err := h.invites.Create(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team"), body.Role)
	created := createdInviteDTO{ID: invite.Link.ID, Role: invite.Link.Role, ExpiresAt: invite.Link.ExpiresAt, URL: invite.URL}
	apispec.Respond(c, http.StatusCreated, created, err)
}

func (h InvitesHandler) List(c *gin.Context) {
	load := func() ([]entities.AccountLink, error) {
		return h.invites.List(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team"))
	}
	apispec.RespondWindow(c, load, inviteKey, toInviteDTO)
}

func (h InvitesHandler) Revoke(c *gin.Context) {
	if err := h.invites.Revoke(c.Request.Context(), requestctx.ActorFrom(c), c.Param("team"), c.Param("id")); err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func RegisterInvitesRoutes(api httpserver.API, h InvitesHandler) {
	api.Handle(
		apispec.Operation{ID: "createInvite", Method: http.MethodPost, Path: "/teams/:team/invites", Summary: "Create a one-time invite link for a team and role; the url is shown only once",
			Scope: scope.TeamsAdmin, Params: teamParams{}, Body: createInviteDTO{}, Status: http.StatusCreated, Response: createdInviteDTO{}, Handler: h.Create},
		apispec.Operation{ID: "listInvites", Method: http.MethodGet, Path: "/teams/:team/invites", Summary: "List a team's pending invites, newest first",
			Scope: scope.TeamsAdmin, Params: listInvitesParams{}, Status: http.StatusOK, Response: apispec.Page[inviteDTO]{}, Handler: h.List},
		apispec.Operation{ID: "revokeInvite", Method: http.MethodDelete, Path: "/teams/:team/invites/:id", Summary: "Revoke a pending invite so its link stops working",
			Scope: scope.TeamsAdmin, Params: inviteParams{}, Status: http.StatusNoContent, Handler: h.Revoke},
	)
}
