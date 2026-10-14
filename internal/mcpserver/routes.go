package mcpserver

import (
	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/internal/httpserver"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/gin-gonic/gin"
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
