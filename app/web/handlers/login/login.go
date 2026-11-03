package login

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/identity/usecases/accounts"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/pkg/config"
)

var errorMessages = map[string]string{
	"state":       "Your sign-in link expired. Please try again.",
	"github":      "GitHub didn't confirm who you are. Please try again.",
	"demo":        "The demo isn't open right now. Sign in with GitHub instead.",
	"demo-busy":   "Too many demo visits from your network this hour. Try again later.",
	"password":    "Wrong username or password.",
	"throttled":   "Too many attempts, try again later.",
	"unavailable": "Something went wrong on our side. Try again in a moment.",
}

type data struct {
	Error     string
	Next      string
	RawNext   string
	Demo      bool
	Passwords bool
}

type LoginHandler struct {
	renderer  *web.Renderer
	demo      bool
	passwords bool
}

func NewLoginHandler(cfg config.Config, mode accounts.Mode, renderer *web.Renderer) LoginHandler {
	return LoginHandler{renderer: renderer, demo: cfg.Demo.Enabled, passwords: !mode.GitHub}
}

func (h LoginHandler) Show(c *gin.Context) {
	d := data{
		Error: errorMessages[c.Query("error")], Next: url.QueryEscape(c.Query("next")), RawNext: c.Query("next"),
		Demo: h.demo, Passwords: h.passwords,
	}
	h.renderer.Render(c, http.StatusOK, "login", web.Page{Title: "Sign in", Data: d})
}

func RegisterLoginRoutes(public httpserver.Public, h LoginHandler) {
	public.GET("/login", h.Show)
}
