package guard

import (
	"context"
	"crypto/subtle"
	"net/http"
	"net/url"

	"github.com/adrianozp/platform/app/identity/entities"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/gin-gonic/gin"
)

const (
	SessionCookie = "platform_session"
	CSRFHeader    = "X-CSRF-Token"
	CSRFField     = "_csrf"
)

type Authenticator interface {
	Authenticate(ctx context.Context, token string) (entities.Authenticated, error)
}

var errInvalidCSRF = apperror.New(apperror.CodeInvalidCSRFToken, "missing or invalid CSRF token, reload the page")

func NewGuards(auth Authenticator) httpserver.Guards {
	return httpserver.Guards{API: apiGuard(auth), Web: webGuard(auth)}
}

func apiGuard(auth Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := authenticate(c, auth); err != nil {
			httperror.Abort(c, err)
			return
		}
		if err := checkCSRF(c); err != nil {
			httperror.Abort(c, err)
		}
	}
}

func webGuard(auth Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := authenticate(c, auth); err != nil {
			redirectToLogin(c)
			return
		}
		if err := checkCSRF(c); err != nil {
			httperror.Abort(c, err)
		}
	}
}

func authenticate(c *gin.Context, auth Authenticator) error {
	token, _ := c.Cookie(SessionCookie)
	authenticated, err := auth.Authenticate(c.Request.Context(), token)
	if err != nil {
		return err
	}
	requestctx.SetViewer(c, requestctx.Viewer{
		Actor: authenticated.Actor(), Name: authenticated.User.Name,
		AvatarURL: authenticated.User.AvatarURL, CSRFToken: authenticated.CSRFToken,
	})
	return nil
}

func checkCSRF(c *gin.Context) error {
	if isSafe(c.Request.Method) {
		return nil
	}
	sent := c.GetHeader(CSRFHeader)
	if sent == "" {
		sent = c.PostForm(CSRFField)
	}
	expected := requestctx.ViewerFrom(c).CSRFToken
	if sent == "" || subtle.ConstantTimeCompare([]byte(sent), []byte(expected)) != 1 {
		return errInvalidCSRF
	}
	return nil
}

func isSafe(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func redirectToLogin(c *gin.Context) {
	target := "/login?next=" + url.QueryEscape(c.Request.URL.RequestURI())
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", target)
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Redirect(http.StatusSeeOther, target)
	c.Abort()
}
