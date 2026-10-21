package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	observe "github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

const (
	graphWindowMs   = 10 * 60 * 1000
	repository      = "klzn-labs/kalzone"
	deployedFromRef = "klzn-labs/kalzone-infra"
)

var panels = []int{
	observe.PanelRequests, observe.PanelLatency, observe.PanelMemory, observe.PanelCPU,
	observe.PanelRouteRequests, observe.PanelRouteLatency,
}

type Grafana interface {
	Enabled() bool
}

type Platform interface {
	Component(ctx context.Context, name string) (deploy.PlatformComponent, error)
}

type Row struct {
	Deploy   deploy.PlatformDeploy
	GraphURL string
}

type pageData struct {
	Component    deploy.PlatformComponent
	Stale        bool
	Rows         []Row
	DashboardURL string
	DashboardUID string
	Grafana      bool
	Panels       []int
	LogsPanel    int
	Repository   string
	DeployedFrom string
}

type PlatformHandler struct {
	renderer *web.Renderer
	platform Platform
	grafana  Grafana
}

func NewPlatformHandler(renderer *web.Renderer, platform Platform, grafana Grafana) PlatformHandler {
	return PlatformHandler{renderer: renderer, platform: platform, grafana: grafana}
}

func (h PlatformHandler) Show(c *gin.Context) {
	component, err := h.platform.Component(c.Request.Context(), c.Param("name"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "platform", web.Page{Title: "Kalzone " + component.Name, Nav: "services", Data: h.page(component)})
}

func (h PlatformHandler) page(c deploy.PlatformComponent) pageData {
	return pageData{
		Component: c, Stale: c.Stale(clock.Now()), Rows: rows(c),
		DashboardURL: DashboardURL(c.Name), DashboardUID: observe.PlatformDashboardUID(c.Name),
		Grafana: h.grafana.Enabled(), Panels: panels, LogsPanel: observe.PanelLogs,
		Repository: repository, DeployedFrom: deployedFromRef,
	}
}

func rows(c deploy.PlatformComponent) []Row {
	out := make([]Row, 0, len(c.History))
	for _, d := range c.History {
		out = append(out, Row{Deploy: d, GraphURL: GraphURL(c.Name, d)})
	}
	return out
}

func DashboardURL(name string) string {
	return "/grafana/d/" + url.PathEscape(observe.PlatformDashboardUID(name))
}

func GraphURL(name string, d deploy.PlatformDeploy) string {
	return fmt.Sprintf("%s?time=%d&time.window=%d", DashboardURL(name), d.At.UnixMilli(), graphWindowMs)
}

func RegisterPlatformRoutes(routes httpserver.Web, h PlatformHandler) {
	routes.GET("/platform/:name", h.Show)
}
