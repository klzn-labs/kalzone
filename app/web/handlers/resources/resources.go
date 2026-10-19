package resources

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/app/web/handlers/queues"
	"github.com/klzn-labs/kalzone/internal/httpserver"
)

const queueKind = "queue"

type QueueSection interface {
	Rows(ctx context.Context, environment string) ([]queues.Row, error)
}

type Section struct {
	Kind    string
	Title   string
	Noun    string
	NewHref string
	Count   int
	Queues  []queues.Row
}

type listView struct {
	Sections    []Section
	Environment string
	Kind        string
}

type ResourcesHandler struct {
	renderer *web.Renderer
	queues   QueueSection
}

func NewResourcesHandler(renderer *web.Renderer, queues QueueSection) ResourcesHandler {
	return ResourcesHandler{renderer: renderer, queues: queues}
}

func (h ResourcesHandler) List(c *gin.Context) {
	environment, kind := c.Query("environment"), c.Query("kind")
	sections := []Section{}
	if kind == "" || kind == queueKind {
		section, err := h.queueSection(c.Request.Context(), environment)
		if err != nil {
			web.ErrorPage(c, h.renderer, err)
			return
		}
		sections = append(sections, section)
	}
	h.renderer.Render(c, http.StatusOK, "resources", web.Page{Title: "Resources", Nav: "resources", Data: listView{Sections: sections, Environment: environment, Kind: kind}})
}

func (h ResourcesHandler) queueSection(ctx context.Context, environment string) (Section, error) {
	rows, err := h.queues.Rows(ctx, environment)
	if err != nil {
		return Section{}, err
	}
	return Section{Kind: queueKind, Title: "Queues", Noun: "queue", NewHref: "/resources/new/queue", Count: len(rows), Queues: rows}, nil
}

func RegisterResourcesRoutes(routes httpserver.Web, h ResourcesHandler) {
	routes.GET("/resources", h.List)
	routes.GET("/resources/new", redirectTo("/resources/new/queue"))
	routes.GET("/queues", redirectTo("/resources?kind=queue"))
	routes.GET("/queues/*rest", redirectOldQueue)
}

func redirectOldQueue(c *gin.Context) {
	switch rest := c.Param("rest"); rest {
	case "", "/":
		redirectTo("/resources?kind=queue")(c)
	case "/new":
		redirectTo("/resources/new/queue")(c)
	default:
		redirectTo("/resources/queues" + rest)(c)
	}
}
