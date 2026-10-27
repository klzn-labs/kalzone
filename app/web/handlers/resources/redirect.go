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

func redirectToKind(c *gin.Context, info kindInfo) {
	query := c.Request.URL.Query()
	query.Del("kind")
	c.Redirect(http.StatusMovedPermanently, withQuery(info.Href, query.Encode()))
}

func redirectOldQueue(c *gin.Context) {
	switch rest := c.Param("rest"); rest {
	case "", "/":
		redirectTo(kinds[queueKind].Href)(c)
	case "/new":
		redirectTo("/resources/new/queue")(c)
	default:
		redirectTo("/resources/queues" + rest)(c)
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
