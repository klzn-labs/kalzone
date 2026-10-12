package tokens

import (
	"net/http"

	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/gin-gonic/gin"
)

func (h TokensHandler) Whoami(c *gin.Context) {
	who, err := h.tokens.Whoami(c.Request.Context(), requestctx.ActorFrom(c))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, toWhoami(who))
}
