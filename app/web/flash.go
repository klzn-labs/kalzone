package web

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

const flashCookie = "platform_flash"

type Flash struct {
	Message string
	Error   bool
}

func SetFlash(c *gin.Context, f Flash) {
	kind := "ok"
	if f.Error {
		kind = "error"
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(flashCookie, url.QueryEscape(kind+":"+f.Message), 60, "/", "", false, true)
}

func TakeFlash(c *gin.Context) Flash {
	raw, err := c.Cookie(flashCookie)
	if err != nil || raw == "" {
		return Flash{}
	}
	c.SetCookie(flashCookie, "", -1, "/", "", false, true)
	decoded, _ := url.QueryUnescape(raw)
	kind, message, _ := strings.Cut(decoded, ":")
	return Flash{Message: message, Error: kind == "error"}
}
