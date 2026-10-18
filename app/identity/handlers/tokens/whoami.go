package tokens

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/requestctx"
)

func (h TokensHandler) Whoami(c *gin.Context) {
	who, err := h.tokens.Whoami(c.Request.Context(), requestctx.ActorFrom(c))
	apispec.Respond(c, http.StatusOK, toWhoami(who), err)
}
