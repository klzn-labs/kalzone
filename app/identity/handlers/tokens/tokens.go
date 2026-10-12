package tokens

import (
	"context"
	"net/http"

	"github.com/adrianozp/platform/app/identity/entities"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/actor"
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

func (h TokensHandler) List(c *gin.Context) {
	list, err := h.list(c)
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]TokenDTO, 0, len(list))
	for _, t := range list {
		result = append(result, ToDTO(t))
	}
	c.JSON(http.StatusOK, gin.H{"tokens": result})
}

func (h TokensHandler) list(c *gin.Context) ([]entities.AccessToken, error) {
	a := requestctx.ActorFrom(c)
	if c.Query("all") == "true" {
		return h.tokens.ListAll(c.Request.Context(), a)
	}
	return h.tokens.List(c.Request.Context(), a)
}

func (h TokensHandler) Create(c *gin.Context) {
	var body createDTO
	if !httperror.BindJSON(c, &body) {
		return
	}
	issued, err := h.tokens.Create(c.Request.Context(), requestctx.ActorFrom(c), body.toEntity())
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusCreated, issuedDTO{TokenDTO: ToDTO(issued.Token), Token: issued.Secret})
}

func (h TokensHandler) Revoke(c *gin.Context) {
	if err := h.tokens.Revoke(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id")); err != nil {
		httperror.Abort(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func RegisterTokensRoutes(api httpserver.API, h TokensHandler) {
	api.GET("/whoami", h.Whoami)
	api.GET("/tokens", h.List)
	api.POST("/tokens", h.Create)
	api.DELETE("/tokens/:id", h.Revoke)
}
