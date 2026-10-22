package observability

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	envconfig "github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/observe/usecases/logs"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

const (
	logLines       = 100
	longestLogs    = 24 * time.Hour
	deploysScanned = 50
)

type Services interface {
	GetFor(ctx context.Context, a actor.Actor, slug string) (catalog.Service, error)
}

type Configs interface {
	Current(ctx context.Context, serviceSlug, environment string) (envconfig.Revision, error)
}

type Metrics interface {
	Read(ctx context.Context, a actor.Actor, serviceSlug, environment string, w entities.Window) (entities.Metrics, error)
}

type Logs interface {
	Read(ctx context.Context, a actor.Actor, serviceSlug, environment string, q logs.Query) ([]entities.LogLine, error)
}

type Limits interface {
	ResponseLimit(limits catalog.Limits) int
}

type Deployments interface {
	History(ctx context.Context, serviceSlug, environment string, limit int) ([]deploy.Deployment, error)
}

type Grafana interface {
	Enabled() bool
}

type Deps struct {
	Services    Services
	Configs     Configs
	Metrics     Metrics
	Logs        Logs
	Limits      Limits
	Deployments Deployments
	Grafana     Grafana
}

type ObservabilityHandler struct {
	renderer *web.Renderer
	deps     Deps
}

func NewObservabilityHandler(renderer *web.Renderer, deps Deps) ObservabilityHandler {
	return ObservabilityHandler{renderer: renderer, deps: deps}
}

func (h ObservabilityHandler) Show(c *gin.Context) {
	s := state{
		base: c.Request.URL.Path, window: entities.WindowNamed(c.Query("window")),
		signal: entities.SignalNamed(c.Query("signal")), logs: c.Query("logs"),
	}
	v, err := h.build(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"), s)
	if errors.Is(err, apperror.NotFound("")) {
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		h.renderer.Fragment(c, http.StatusOK, "environment", "observability-unavailable", nil)
		return
	}
	h.renderer.Fragment(c, http.StatusOK, "environment", "observability", v)
}

func (h ObservabilityHandler) build(ctx context.Context, a actor.Actor, slug, env string, s state) (View, error) {
	in, err := h.inputs(ctx, a, slug, env, s)
	if err != nil {
		return View{}, err
	}
	v := viewOf(in)
	v.Logs, v.LogsNote = h.logs(ctx, a, slug, env, s)
	if a.Admin && h.deps.Grafana.Enabled() {
		v.Dashboard = "/grafana/d/platform-" + slug + "?var-env=" + env
	}
	return v, nil
}

func (h ObservabilityHandler) inputs(ctx context.Context, a actor.Actor, slug, env string, s state) (inputs, error) {
	service, err := h.deps.Services.GetFor(ctx, a, slug)
	if err != nil {
		return inputs{}, err
	}
	revision, err := h.deps.Configs.Current(ctx, slug, env)
	if err != nil {
		return inputs{}, err
	}
	m, err := h.deps.Metrics.Read(ctx, a, slug, env, s.window)
	if err != nil {
		return inputs{}, err
	}
	history, err := h.deps.Deployments.History(ctx, slug, env, deploysScanned)
	l := limits{latencyMS: float64(h.deps.Limits.ResponseLimit(service.Limits)), memoryMB: float64(revision.Spec.MemoryMB)}
	return inputs{state: s, metrics: m, limits: l, history: history}, err
}

func (h ObservabilityHandler) logs(ctx context.Context, a actor.Actor, slug, env string, s state) ([]LogRow, string) {
	q := logs.Query{Since: min(s.window.Span, longestLogs), Limit: logLines, Errors: s.logs == "errors"}
	lines, err := h.deps.Logs.Read(ctx, a, slug, env, q)
	if err != nil {
		return nil, "Logs are unavailable right now."
	}
	return logRows(lines), ""
}

func RegisterObservabilityRoutes(routes httpserver.Web, h ObservabilityHandler) {
	routes.GET("/services/:service/env/:env/observability", h.Show)
}
