package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/adrianozp/platform/app/identity/entities"
	"github.com/adrianozp/platform/app/identity/handlers/guard"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/gin-gonic/gin"
)

const (
	stateCookie  = "platform_oauth_state"
	nextCookie   = "platform_oauth_next"
	callbackPath = "/auth/github/callback"
	stateMaxAge  = 600
)

type SignInStarter interface {
	AuthorizeURL(state, redirectURI string) string
}

type SignIner interface {
	SignIn(ctx context.Context, code, redirectURI string) (entities.IssuedSession, error)
}

type SignOuter interface {
	SignOut(ctx context.Context, token string) error
}

type AuthHandler struct {
	starter     SignInStarter
	signIner    SignIner
	signOuter   SignOuter
	redirectURI string
	secure      bool
}

func NewAuthHandler(cfg config.Config, starter SignInStarter, signIner SignIner, signOuter SignOuter) AuthHandler {
	return AuthHandler{
		starter: starter, signIner: signIner, signOuter: signOuter,
		redirectURI: strings.TrimRight(cfg.BaseURL, "/") + callbackPath, secure: cfg.SecureCookies(),
	}
}

func (h AuthHandler) Start(c *gin.Context) {
	state, err := entities.NewToken()
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	h.setCookie(c, stateCookie, state, stateMaxAge)
	h.setCookie(c, nextCookie, safeNext(c.Query("next")), stateMaxAge)
	c.Redirect(http.StatusFound, h.starter.AuthorizeURL(state, h.redirectURI))
}

func (h AuthHandler) Callback(c *gin.Context) {
	expected, _ := c.Cookie(stateCookie)
	next, _ := c.Cookie(nextCookie)
	h.setCookie(c, stateCookie, "", -1)
	h.setCookie(c, nextCookie, "", -1)
	if expected == "" || c.Query("state") != expected {
		c.Redirect(http.StatusSeeOther, "/login?error=state")
		return
	}
	issued, err := h.signIner.SignIn(c.Request.Context(), c.Query("code"), h.redirectURI)
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/login?error=github")
		return
	}
	h.setCookie(c, guard.SessionCookie, issued.Token, int(issued.ExpiresAt.Sub(clock.Now()).Seconds()))
	c.Redirect(http.StatusSeeOther, safeNext(next))
}

func (h AuthHandler) Logout(c *gin.Context) {
	token, _ := c.Cookie(guard.SessionCookie)
	_ = h.signOuter.SignOut(c.Request.Context(), token)
	h.setCookie(c, guard.SessionCookie, "", -1)
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", "/login")
		c.Status(http.StatusNoContent)
		return
	}
	c.Redirect(http.StatusSeeOther, "/login")
}

func (h AuthHandler) setCookie(c *gin.Context, name, value string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, maxAge, "/", "", h.secure, true)
}

func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

func RegisterAuthRoutes(public httpserver.Public, web httpserver.Web, h AuthHandler) {
	public.GET("/auth/github", h.Start)
	public.GET(callbackPath, h.Callback)
	web.POST("/logout", h.Logout)
}
