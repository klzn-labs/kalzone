package mcpserver

import (
	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/pkg/config"
	"go.uber.org/fx"
)

const Path = "/mcp"

func RegisterMCPRoutes(lc fx.Lifecycle, public httpserver.Public, guards httpserver.Guards, engine *gin.Engine, catalog *apispec.Catalog, cfg config.Config) {
	h := &Handler{engine: engine}
	public.POST(Path, guards.Token, h.Serve)
	public.GET(Path, guards.Token, h.Serve)
	public.DELETE(Path, guards.Token, h.Serve)
	lc.Append(fx.StartHook(func() error { return h.load(catalog, cfg.BaseURL) }))
}
