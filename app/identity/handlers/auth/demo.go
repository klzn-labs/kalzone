package auth

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/ratelimit"
	"github.com/klzn-labs/kalzone/pkg/config"
)

type DemoEnterer interface {
	EnterDemo(ctx context.Context) (entities.IssuedSession, error)
}

type DemoHandler struct {
	enterer DemoEnterer
	limiter *ratelimit.Limiter
	secure  bool
}

func NewDemoHandler(cfg config.Config, enterer DemoEnterer) DemoHandler {
	return DemoHandler{enterer: enterer, limiter: ratelimit.PerHour(cfg.Demo.PerIPHour), secure: cfg.SecureCookies()}
}

func (h DemoHandler) Enter(c *gin.Context) {
	if !h.limiter.Allow(c.ClientIP()) {
		c.Redirect(http.StatusSeeOther, "/login?error=demo-busy")
		return
	}
	issued, err := h.enterer.EnterDemo(c.Request.Context())
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/login?error=demo")
		return
	}
	SetSessionCookie(c, h.secure, issued)
	c.Redirect(http.StatusSeeOther, "/")
}

func RegisterDemoRoutes(public httpserver.Public, h DemoHandler) {
	public.POST("/demo", h.Enter)
}
