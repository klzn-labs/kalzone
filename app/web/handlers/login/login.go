package login

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/pkg/config"
)

var errorMessages = map[string]string{
	"state":     "Your sign-in link expired. Please try again.",
	"github":    "GitHub didn't confirm who you are. Please try again.",
	"demo":      "The demo isn't open right now. Sign in with GitHub instead.",
	"demo-busy": "Too many demo visits from your network this hour. Try again later.",
}

type data struct {
	Error string
	Next  string
	Demo  bool
}

type LoginHandler struct {
	renderer *web.Renderer
	demo     bool
}

func NewLoginHandler(cfg config.Config, renderer *web.Renderer) LoginHandler {
	return LoginHandler{renderer: renderer, demo: cfg.Demo.Enabled}
}

func (h LoginHandler) Show(c *gin.Context) {
	page := web.Page{Title: "Sign in", Data: data{Error: errorMessages[c.Query("error")], Next: url.QueryEscape(c.Query("next")), Demo: h.demo}}
	h.renderer.Render(c, http.StatusOK, "login", page)
}

func RegisterLoginRoutes(public httpserver.Public, h LoginHandler) {
	public.GET("/login", h.Show)
}
