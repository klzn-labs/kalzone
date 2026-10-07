package login

import (
	"net/http"
	"net/url"

	"github.com/adrianozp/platform/app/web"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/gin-gonic/gin"
)

var errorMessages = map[string]string{
	"state":  "Your sign-in link expired. Please try again.",
	"github": "GitHub didn't confirm who you are. Please try again.",
}

type data struct {
	Error string
	Next  string
}

type LoginHandler struct {
	renderer *web.Renderer
}

func NewLoginHandler(renderer *web.Renderer) LoginHandler {
	return LoginHandler{renderer: renderer}
}

func (h LoginHandler) Show(c *gin.Context) {
	page := web.Page{Title: "Sign in", Data: data{Error: errorMessages[c.Query("error")], Next: url.QueryEscape(c.Query("next"))}}
	h.renderer.Render(c, http.StatusOK, "login", page)
}

func RegisterLoginRoutes(public httpserver.Public, h LoginHandler) {
	public.GET("/login", h.Show)
}
