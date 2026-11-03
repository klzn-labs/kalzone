package people

import (
	"net/http"

	"github.com/gin-gonic/gin"
	identity "github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/handlers/guard"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var errNoPasswordPage = apperror.NotFound("page not found")

type passwordData struct {
	Error string
}

func (h PeopleHandler) ShowPassword(c *gin.Context) {
	if !h.passwordPage(c) {
		return
	}
	h.renderPassword(c, http.StatusOK, "")
}

func (h PeopleHandler) passwordPage(c *gin.Context) bool {
	if h.passwords && !requestctx.ActorFrom(c).Guest {
		return true
	}
	web.ErrorPage(c, h.renderer, errNoPasswordPage)
	return false
}

func (h PeopleHandler) ChangePassword(c *gin.Context) {
	if !h.passwordPage(c) {
		return
	}
	session, _ := c.Cookie(guard.SessionCookie)
	err := h.accounts.ChangePassword(c.Request.Context(), requestctx.ActorFrom(c),
		c.PostForm("current"), c.PostForm("password"), c.PostForm("confirm"), identity.HashToken(session))
	if err == nil {
		web.Done(c, "/people", "Your password was changed. Your other sessions were signed out.")
		return
	}
	status := http.StatusUnprocessableEntity
	if apperror.CodeOf(err) == apperror.CodeInternal {
		status = http.StatusInternalServerError
	}
	h.renderPassword(c, status, web.UserMessage(err))
}

func (h PeopleHandler) renderPassword(c *gin.Context, status int, problem string) {
	page := web.Page{Title: "Change password", Nav: "people", Data: passwordData{Error: problem}}
	h.renderer.Render(c, status, "password", page)
}
