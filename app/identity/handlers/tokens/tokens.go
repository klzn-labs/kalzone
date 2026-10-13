package tokens

import (
	"context"
	"net/http"

	"github.com/adrianozp/platform/app/identity/entities"
	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/scope"
	"github.com/gin-gonic/gin"
)

type Tokens interface {
	List(ctx context.Context, a actor.Actor) ([]entities.AccessToken, error)
	ListAll(ctx context.Context, a actor.Actor) ([]entities.AccessToken, error)
	Create(ctx context.Context, a actor.Actor, d entities.TokenDraft) (entities.IssuedToken, error)
	Revoke(ctx context.Context, a actor.Actor, id string) error
	Whoami(ctx context.Context, a actor.Actor) (entities.Whoami, error)
}

type TokensHandler struct {
	tokens Tokens
}

func NewTokensHandler(tokens Tokens) TokensHandler {
	return TokensHandler{tokens: tokens}
}

type listParams struct {
	apispec.ListParams
	All bool `query:"all" description:"every person's tokens; admins in a browser session only"`
}

type tokenParams struct {
	ID string `path:"id"`
}

func (h TokensHandler) List(c *gin.Context) {
	apispec.RespondWindow(c, func() ([]entities.AccessToken, error) { return h.list(c) }, tokenKey, ToDTO)
}

func (h TokensHandler) list(c *gin.Context) ([]entities.AccessToken, error) {
	a := requestctx.ActorFrom(c)
	if c.Query("all") == "true" {
		return h.tokens.ListAll(c.Request.Context(), a)
	}
	return h.tokens.List(c.Request.Context(), a)
}

func tokenKey(t entities.AccessToken) string {
	return t.ID
}

func (h TokensHandler) Create(c *gin.Context) {
	var body createDTO
	if !httperror.BindStrict(c, &body) {
		return
	}
	issued, err := h.tokens.Create(c.Request.Context(), requestctx.ActorFrom(c), body.toEntity())
	apispec.Respond(c, http.StatusCreated, issuedDTO{TokenDTO: ToDTO(issued.Token), Token: issued.Secret}, err)
}

func (h TokensHandler) Revoke(c *gin.Context) {
	if err := h.tokens.Revoke(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id")); err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func RegisterTokensRoutes(api httpserver.API, h TokensHandler) {
	api.Handle(
		apispec.Operation{ID: "getWhoami", Method: http.MethodGet, Path: "/whoami", Summary: "Who this token or session acts for, with its scopes and teams",
			Status: http.StatusOK, Response: whoamiDTO{}, Handler: h.Whoami},
		apispec.Operation{ID: "listTokens", Method: http.MethodGet, Path: "/tokens", Summary: "List your access tokens, newest first",
			Scope: scope.TokensAdmin, Params: listParams{}, Status: http.StatusOK, Response: apispec.Page[TokenDTO]{}, Handler: h.List},
		apispec.Operation{ID: "createToken", Method: http.MethodPost, Path: "/tokens", Summary: "Create an access token; its secret is shown once",
			Scope: scope.TokensAdmin, Body: createDTO{}, Status: http.StatusCreated, Response: issuedDTO{}, Handler: h.Create},
		apispec.Operation{ID: "revokeToken", Method: http.MethodDelete, Path: "/tokens/:id", Summary: "Revoke an access token",
			Scope: scope.TokensAdmin, Params: tokenParams{}, Status: http.StatusNoContent, Handler: h.Revoke},
	)
}
