package tokens

import (
	"net/http"

	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/gin-gonic/gin"
)

func (h TokensHandler) Whoami(c *gin.Context) {
	who, err := h.tokens.Whoami(c.Request.Context(), requestctx.ActorFrom(c))
	apispec.Respond(c, http.StatusOK, toWhoami(who), err)
}
