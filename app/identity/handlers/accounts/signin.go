package accounts

import (
	"errors"
	"log"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/identity/handlers/auth"
	usecase "github.com/klzn-labs/kalzone/app/identity/usecases/accounts"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

func (h PagesHandler) SignInWithPassword(c *gin.Context) {
	if !h.available(c) {
		return
	}
	next := c.PostForm("next")
	issued, err := h.signIn.SignIn(c.Request.Context(), c.PostForm("username"), c.PostForm("password"), c.ClientIP())
	if err != nil {
		c.Redirect(http.StatusSeeOther, loginAgain(signInProblem(err), next))
		return
	}
	auth.SetSessionCookie(c, h.secure, issued)
	c.Redirect(http.StatusSeeOther, auth.SafeNext(next))
}

func signInProblem(err error) string {
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeSignInFailed {
		log.Printf("password sign-in failed: %v", err)
		return "unavailable"
	}
	if appErr == usecase.ErrThrottled {
		return "throttled"
	}
	return "password"
}

func loginAgain(problem, next string) string {
	query := url.Values{"error": {problem}}
	if next != "" {
		query.Set("next", next)
	}
	return "/login?" + query.Encode()
}
