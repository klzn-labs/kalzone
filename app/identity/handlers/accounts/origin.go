package accounts

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/web"
)

type wrongOriginData struct {
	BaseURL string
}

func SameOrigin(baseURL string, renderer *web.Renderer) gin.HandlerFunc {
	expected := originOf(baseURL)
	page := web.Page{Title: "Wrong address", Data: wrongOriginData{BaseURL: strings.TrimSuffix(baseURL, "/")}}
	return func(c *gin.Context) {
		if crossSite(c.GetHeader("Sec-Fetch-Site")) || foreignOrigin(c.GetHeader("Origin"), expected) {
			renderer.Render(c, http.StatusForbidden, "wrong_origin", page)
			c.Abort()
		}
	}
}

func crossSite(fetchSite string) bool {
	return fetchSite != "" && fetchSite != "same-origin" && fetchSite != "none"
}

func foreignOrigin(origin, expected string) bool {
	return origin != "" && originOf(origin) != expected
}

func originOf(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "invalid:" + raw
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}
