package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/internal/httpserver"
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
