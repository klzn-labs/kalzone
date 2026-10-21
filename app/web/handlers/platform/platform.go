package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

const (
	dashboardUID    = "kalzone-http"
	graphWindowMs   = 10 * 60 * 1000
	repository      = "klzn-labs/kalzone"
	deployedFromRef = "klzn-labs/kalzone-infra"
)

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
	Repository   string
	DeployedFrom string
}

type PlatformHandler struct {
	renderer *web.Renderer
	platform Platform
}

func NewPlatformHandler(renderer *web.Renderer, platform Platform) PlatformHandler {
	return PlatformHandler{renderer: renderer, platform: platform}
}

func (h PlatformHandler) Show(c *gin.Context) {
	component, err := h.platform.Component(c.Request.Context(), c.Param("name"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	d := pageData{
		Component: component, Stale: component.Stale(clock.Now()), Rows: rows(component),
		DashboardURL: DashboardURL(component.Name), Repository: repository, DeployedFrom: deployedFromRef,
	}
	h.renderer.Render(c, http.StatusOK, "platform", web.Page{Title: "Kalzone " + component.Name, Nav: "services", Data: d})
}

func rows(c deploy.PlatformComponent) []Row {
	out := make([]Row, 0, len(c.History))
	for _, d := range c.History {
		out = append(out, Row{Deploy: d, GraphURL: GraphURL(c.Name, d)})
	}
	return out
}

func DashboardURL(name string) string {
	return "/grafana/d/" + dashboardUID + "?var-service=" + url.QueryEscape(name)
}

func GraphURL(name string, d deploy.PlatformDeploy) string {
	return fmt.Sprintf("%s&time=%d&time.window=%d", DashboardURL(name), d.At.UnixMilli(), graphWindowMs)
}

func RegisterPlatformRoutes(routes httpserver.Web, h PlatformHandler) {
	routes.GET("/platform/:name", h.Show)
}
