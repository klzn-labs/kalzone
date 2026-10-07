package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/gin-gonic/gin"
)

//go:embed templates static
var assets embed.FS

func RegisterStaticRoutes(public httpserver.Public) {
	static, _ := fs.Sub(assets, "static")
	public.Group("/static", cacheStatic).StaticFS("/", http.FS(static))
}

func cacheStatic(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=86400")
	c.Next()
}
