package resources

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func redirectTo(target string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, withQuery(target, c.Request.URL.RawQuery))
	}
}

func withQuery(target, query string) string {
	if query == "" {
		return target
	}
	if strings.Contains(target, "?") {
		return target + "&" + query
	}
	return target + "?" + query
}
