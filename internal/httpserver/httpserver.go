package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/internal/httpserver/wait"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"
)

var privateNetworks = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "::1/128", "fc00::/7"}

func NewEngine() *gin.Engine {
	engine := gin.New()
	_ = engine.SetTrustedProxies(privateNetworks)
	engine.Use(gin.Recovery(), requestLogger(), securityHeaders(), metricsMiddleware())
	engine.GET("/healthz", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	engine.GET("/metrics", gin.WrapH(promhttp.Handler()))
	return engine
}

func Run(lc fx.Lifecycle, cfg config.Config, engine *gin.Engine, drain *wait.Drain) {
	server := &http.Server{Addr: cfg.HTTP.Addr, Handler: engine, ReadHeaderTimeout: 10 * time.Second}
	server.RegisterOnShutdown(drain.Close)
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go serve(server)
			return nil
		},
		OnStop: server.Shutdown,
	})
}

func serve(server *http.Server) {
	log.Printf("listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("http server: %v", err)
	}
}

func requestLogger() gin.HandlerFunc {
	return gin.LoggerWithConfig(gin.LoggerConfig{SkipPaths: []string{"/healthz", "/metrics"}, Formatter: logLine})
}

func logLine(p gin.LogFormatterParams) string {
	return fmt.Sprintf("[GIN] %v | %3d | %13v | %15s | %-7s %#v\n%s",
		p.TimeStamp.Format("2006/01/02 - 15:04:05"), p.StatusCode, p.Latency, p.ClientIP, p.Method, withoutLinkToken(p.Path), p.ErrorMessage)
}

func withoutLinkToken(path string) string {
	if strings.HasPrefix(path, "/join/") {
		return "/join/[redacted]"
	}
	return path
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		c.Next()
	}
}
